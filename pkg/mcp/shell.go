package mcp

import (
	"fmt"
	"path/filepath"
	"strings"
)

// shellKind classifies a shell so a secret tag can be translated to the
// reference that shell expands.
type shellKind int

const (
	// shellUnknown means the shell is empty or not recognized; a matching
	// secret tag cannot be translated safely.
	shellUnknown shellKind = iota
	// shellPOSIX covers sh and its relatives, where ${NAME} expands.
	shellPOSIX
	// shellPowerShell covers PowerShell, where ${env:NAME} expands.
	shellPowerShell
	// shellCmd covers cmd.exe, where %NAME% expands.
	shellCmd
)

// classifyShell maps a shell name or path to a kind. An empty or unrecognized
// shell is shellUnknown.
func classifyShell(shell string) shellKind {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(shell)))
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "cmd":
		return shellCmd
	case "powershell", "pwsh":
		return shellPowerShell
	case "sh", "bash", "zsh", "dash", "ksh", "ash", "fish", "posix":
		return shellPOSIX
	default:
		return shellUnknown
	}
}

// nativeReference returns the shell-native environment reference for key. Brace
// forms keep key names with non-identifier characters addressable.
func nativeReference(kind shellKind, key string) string {
	switch kind {
	case shellPowerShell:
		return "${env:" + key + "}"
	case shellCmd:
		return "%" + key + "%"
	case shellPOSIX:
		return "${" + key + "}"
	default:
		return ""
	}
}

// referenceTemplate returns the native reference using NAME as a placeholder,
// for advertising the syntax without a concrete key.
func referenceTemplate(kind shellKind) string {
	switch kind {
	case shellPowerShell:
		return "${env:NAME}"
	case shellCmd:
		return "%NAME%"
	case shellPOSIX:
		return "${NAME}"
	default:
		return ""
	}
}

// keySet adapts a resolved key/value map to a membership set.
func keySet(values map[string]string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for key := range values {
		out[key] = struct{}{}
	}
	return out
}

type quoteState int

const (
	quoteNone quoteState = iota
	quoteSingle
	quoteDouble
)

// translateSecretTags replaces every {{KEY}} tag that names an effective key
// with the target shell's native environment reference, so the value is read
// from the environment instead of being placed on the command line. A tag whose
// name is not an effective key passes through unchanged. It returns an error
// when a matching tag cannot be translated safely: the shell is unknown or
// empty, or the tag is inside a single-quoted region where the shell would not
// expand the reference.
func translateSecretTags(kind shellKind, command string, keys map[string]struct{}) (string, error) {
	if len(keys) == 0 || !strings.Contains(command, "{{") {
		return command, nil
	}
	var b strings.Builder
	b.Grow(len(command))
	state := quoteNone
	for i, n := 0, len(command); i < n; {
		if next, handled := consumeShellLiteral(kind, command, i, &state, &b); handled {
			i = next
			continue
		}
		if command[i] == '{' && i+1 < n && command[i+1] == '{' {
			if end := strings.Index(command[i+2:], "}}"); end >= 0 {
				name := command[i+2 : i+2+end]
				if _, ok := keys[name]; ok {
					if kind == shellUnknown {
						return "", fmt.Errorf("cannot use {{%s}} without a known shell: pass the shell argument or use the native environment reference", name)
					}
					if state == quoteSingle {
						return "", fmt.Errorf("cannot use {{%s}} inside single quotes: the shell would not expand the reference; use double quotes or the native environment reference", name)
					}
					b.WriteString(nativeReference(kind, name))
					i = i + 2 + end + 2
					continue
				}
			}
		}
		b.WriteByte(command[i])
		i++
	}
	return b.String(), nil
}

// consumeShellLiteral writes a quote character or escape sequence at index i,
// updates the quote state, and reports the next index. It never consumes a tag.
func consumeShellLiteral(kind shellKind, s string, i int, state *quoteState, b *strings.Builder) (int, bool) {
	n := len(s)
	c := s[i]
	switch kind {
	case shellPowerShell:
		switch *state {
		case quoteSingle:
			if c == '\'' {
				if i+1 < n && s[i+1] == '\'' {
					b.WriteString("''")
					return i + 2, true
				}
				b.WriteByte(c)
				*state = quoteNone
				return i + 1, true
			}
		case quoteDouble:
			if c == '`' && i+1 < n {
				b.WriteByte(c)
				b.WriteByte(s[i+1])
				return i + 2, true
			}
			if c == '"' {
				b.WriteByte(c)
				*state = quoteNone
				return i + 1, true
			}
		default:
			if c == '`' && i+1 < n {
				b.WriteByte(c)
				b.WriteByte(s[i+1])
				return i + 2, true
			}
			if c == '\'' {
				b.WriteByte(c)
				*state = quoteSingle
				return i + 1, true
			}
			if c == '"' {
				b.WriteByte(c)
				*state = quoteDouble
				return i + 1, true
			}
		}
	case shellPOSIX:
		switch *state {
		case quoteSingle:
			if c == '\'' {
				b.WriteByte(c)
				*state = quoteNone
				return i + 1, true
			}
		case quoteDouble:
			if c == '\\' && i+1 < n {
				b.WriteByte(c)
				b.WriteByte(s[i+1])
				return i + 2, true
			}
			if c == '"' {
				b.WriteByte(c)
				*state = quoteNone
				return i + 1, true
			}
		default:
			if c == '\\' && i+1 < n {
				b.WriteByte(c)
				b.WriteByte(s[i+1])
				return i + 2, true
			}
			if c == '\'' {
				b.WriteByte(c)
				*state = quoteSingle
				return i + 1, true
			}
			if c == '"' {
				b.WriteByte(c)
				*state = quoteDouble
				return i + 1, true
			}
		}
	}
	return i, false
}
