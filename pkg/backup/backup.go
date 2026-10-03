// Package backup exports and imports the vault as a passphrase-encrypted file
// so it can be restored independently of the OS keyring.
package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fernandoris/blindenv/pkg/crypto"
	"github.com/fernandoris/blindenv/pkg/db"
)

var (
	// magicV1 is the format produced before shared scopes existed. It carries
	// project and environment secrets only.
	magicV1 = []byte("BLINDENV1\n")
	// magicV2 is the current format, which adds the shared scopes.
	magicV2 = []byte("BLINDENV2\n")
)

const saltSize = 32

// ErrWrongPassphrase is returned when a backup cannot be decrypted.
var ErrWrongPassphrase = errors.New("backup: wrong passphrase or corrupt file")

// ErrUnrecognized is returned when the file is not a BlindEnv backup.
var ErrUnrecognized = errors.New("backup: unrecognized file format")

type data struct {
	Projects []project `json:"projects"`
	Shared   *shared   `json:"shared,omitempty"`
}

type project struct {
	Slug         string            `json:"slug"`
	AllowExecute bool              `json:"allow_execute"`
	Globals      map[string]string `json:"globals"`
	Metadata     map[string]meta   `json:"metadata,omitempty"`
	Environments []environment     `json:"environments"`
}

type environment struct {
	Name     string            `json:"name"`
	Secrets  map[string]string `json:"secrets"`
	Metadata map[string]meta   `json:"metadata,omitempty"`
}

// shared holds the cross-project scopes: the global scope and each shared
// environment scope.
type shared struct {
	Global       map[string]string `json:"global,omitempty"`
	Metadata     map[string]meta   `json:"metadata,omitempty"`
	Environments []environment     `json:"environments,omitempty"`
}

// meta is the backup representation of a definition's metadata. A nil
// Sensitive means sensitive, so older backups and defaults import unchanged.
type meta struct {
	Sensitive *bool  `json:"sensitive,omitempty"`
	Type      string `json:"type,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

func (m meta) secretMeta() db.SecretMeta {
	return db.SecretMeta{Kind: db.SecretKind(m.Type), Hint: m.Hint, Sensitive: m.Sensitive}
}

// metaOf records metadata only when it differs from the sensitive/text/no-hint
// default, so a vault without metadata produces an identical backup.
func metaOf(e db.SecretEntry) (meta, bool) {
	if e.Sensitive && (e.Kind == "" || e.Kind == db.KindText) && e.Hint == "" {
		return meta{}, false
	}
	sensitive := e.Sensitive
	return meta{Sensitive: &sensitive, Type: string(e.Kind), Hint: e.Hint}, true
}

// Export serializes every project, environment and secret, including the
// shared scopes, into a passphrase-encrypted blob.
func Export(ctx context.Context, store *db.Store, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, errors.New("backup: a passphrase is required")
	}
	snapshot, err := snapshotData(ctx, store)
	if err != nil {
		return nil, err
	}
	plain, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("backup: encode: %w", err)
	}
	salt, err := crypto.NewSalt(saltSize)
	if err != nil {
		return nil, err
	}
	ciphertext, err := crypto.Encrypt(crypto.DeriveKey([]byte(passphrase), salt), plain)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(magicV2)+len(salt)+len(ciphertext))
	out = append(out, magicV2...)
	out = append(out, salt...)
	out = append(out, ciphertext...)
	return out, nil
}

// Import restores a backup produced by Export. It decrypts and validates the
// whole file before writing anything, so a wrong passphrase never mutates the
// target vault. Projects that already exist cause an error. Backups produced
// before shared scopes existed (magicV1) are accepted and their project and
// environment secrets are restored into the matching scopes.
func Import(ctx context.Context, store *db.Store, passphrase string, blob []byte) error {
	var rest []byte
	switch {
	case bytes.HasPrefix(blob, magicV2):
		rest = blob[len(magicV2):]
	case bytes.HasPrefix(blob, magicV1):
		rest = blob[len(magicV1):]
	default:
		return ErrUnrecognized
	}
	if len(rest) < saltSize {
		return ErrUnrecognized
	}
	salt, ciphertext := rest[:saltSize], rest[saltSize:]
	plain, err := crypto.Decrypt(crypto.DeriveKey([]byte(passphrase), salt), ciphertext)
	if err != nil {
		return ErrWrongPassphrase
	}
	var snapshot data
	if err := json.Unmarshal(plain, &snapshot); err != nil {
		return fmt.Errorf("backup: decode: %w", err)
	}
	for _, p := range snapshot.Projects {
		if _, err := store.GetProject(ctx, p.Slug); err == nil {
			return fmt.Errorf("backup: project %q already exists", p.Slug)
		} else if !errors.Is(err, db.ErrProjectNotFound) {
			return err
		}
	}
	for _, p := range snapshot.Projects {
		if _, err := store.CreateProject(ctx, p.Slug); err != nil {
			return err
		}
		if err := store.SetAllowExecute(ctx, p.Slug, p.AllowExecute); err != nil {
			return err
		}
		for key, value := range p.Globals {
			if _, err := store.PutSecretMeta(ctx, p.Slug, "", key, value, p.Metadata[key].secretMeta()); err != nil {
				return err
			}
		}
		for _, e := range p.Environments {
			if _, err := store.CreateEnvironment(ctx, p.Slug, e.Name); err != nil {
				return err
			}
			for key, value := range e.Secrets {
				if _, err := store.PutSecretMeta(ctx, p.Slug, e.Name, key, value, e.Metadata[key].secretMeta()); err != nil {
					return err
				}
			}
		}
	}
	if s := snapshot.Shared; s != nil {
		for key, value := range s.Global {
			if _, err := store.PutSecretMeta(ctx, "", "", key, value, s.Metadata[key].secretMeta()); err != nil {
				return err
			}
		}
		for _, e := range s.Environments {
			for key, value := range e.Secrets {
				if _, err := store.PutSecretMeta(ctx, "", e.Name, key, value, e.Metadata[key].secretMeta()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func snapshotData(ctx context.Context, store *db.Store) (*data, error) {
	projects, err := store.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := &data{Projects: make([]project, 0, len(projects))}
	for _, p := range projects {
		globalEntries, err := store.ScopeEntries(ctx, p.Slug, "")
		if err != nil {
			return nil, err
		}
		globals, globalMeta := splitEntries(globalEntries)
		entry := project{
			Slug:         p.Slug,
			AllowExecute: p.AllowExecute,
			Globals:      globals,
			Metadata:     globalMeta,
			Environments: []environment{},
		}
		envs, err := store.ListEnvironments(ctx, p.Slug)
		if err != nil {
			return nil, err
		}
		for _, e := range envs {
			entries, err := store.ScopeEntries(ctx, p.Slug, e.Name)
			if err != nil {
				return nil, err
			}
			secrets, metaMap := splitEntries(entries)
			entry.Environments = append(entry.Environments, environment{Name: e.Name, Secrets: secrets, Metadata: metaMap})
		}
		out.Projects = append(out.Projects, entry)
	}

	snapShared, err := snapshotShared(ctx, store)
	if err != nil {
		return nil, err
	}
	out.Shared = snapShared
	return out, nil
}

func snapshotShared(ctx context.Context, store *db.Store) (*shared, error) {
	globalEntries, err := store.ScopeEntries(ctx, "", "")
	if err != nil {
		return nil, err
	}
	global, globalMeta := splitEntries(globalEntries)
	names, err := store.ListSharedEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	s := &shared{Global: global, Metadata: globalMeta, Environments: []environment{}}
	for _, name := range names {
		entries, err := store.ScopeEntries(ctx, "", name)
		if err != nil {
			return nil, err
		}
		secrets, metaMap := splitEntries(entries)
		if len(secrets) == 0 {
			continue
		}
		s.Environments = append(s.Environments, environment{Name: name, Secrets: secrets, Metadata: metaMap})
	}
	if len(s.Global) == 0 && len(s.Environments) == 0 {
		return nil, nil
	}
	return s, nil
}

// splitEntries turns a scope's entries into the value map and the sparse
// metadata map stored in a backup.
func splitEntries(entries map[string]db.SecretEntry) (map[string]string, map[string]meta) {
	values := make(map[string]string, len(entries))
	var metas map[string]meta
	for key, e := range entries {
		values[key] = e.Value
		if m, ok := metaOf(e); ok {
			if metas == nil {
				metas = make(map[string]meta)
			}
			metas[key] = m
		}
	}
	return values, metas
}
