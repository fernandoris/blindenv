package mcp

import (
	"fmt"
	"strings"
	"testing"
)

func TestClassifyShell(t *testing.T) {
	cases := []struct {
		shell string
		want  shellKind
	}{
		{"", shellUnknown},
		{"bash", shellPOSIX},
		{"/bin/zsh", shellPOSIX},
		{"pwsh.exe", shellPowerShell},
		{"PowerShell", shellPowerShell},
		{"cmd", shellCmd},
		{"cmd.exe", shellCmd},
		{"nushell", shellUnknown},
	}
	for _, tc := range cases {
		if got := classifyShell(tc.shell); got != tc.want {
			t.Errorf("classifyShell(%q) = %d, want %d", tc.shell, got, tc.want)
		}
	}
}

func TestNativeReference(t *testing.T) {
	cases := []struct {
		kind shellKind
		key  string
		want string
	}{
		{shellPowerShell, "API_KEY", "${env:API_KEY}"},
		{shellCmd, "API_KEY", "%API_KEY%"},
		{shellPOSIX, "API_KEY", "${API_KEY}"},
		{shellPowerShell, "MY-KEY", "${env:MY-KEY}"},
		{shellPOSIX, "MY-KEY", "${MY-KEY}"},
	}
	for _, tc := range cases {
		if got := nativeReference(tc.kind, tc.key); got != tc.want {
			t.Errorf("nativeReference(%d, %q) = %q, want %q", tc.kind, tc.key, got, tc.want)
		}
	}
}

func TestTranslateSecretTags(t *testing.T) {
	keys := keySet(map[string]string{"API_KEY": "sk-abcdef123456", "TOKEN": "tok"})
	cases := []struct {
		name    string
		kind    shellKind
		in      string
		want    string
		wantErr bool
	}{
		{"posix bare", shellPOSIX, "curl -H {{API_KEY}}", "curl -H ${API_KEY}", false},
		{"posix double quote", shellPOSIX, `echo "{{API_KEY}}"`, `echo "${API_KEY}"`, false},
		{"posix single quote", shellPOSIX, `echo '{{API_KEY}}'`, "", true},
		{"posix non matching", shellPOSIX, "echo {{OTHER}}", "echo {{OTHER}}", false},
		{"posix escaped", shellPOSIX, `echo \{{API_KEY}}`, `echo \{{API_KEY}}`, false},
		{"posix multiple", shellPOSIX, "{{API_KEY}} {{TOKEN}} {{API_KEY}}", "${API_KEY} ${TOKEN} ${API_KEY}", false},
		{"powershell bare", shellPowerShell, "Write-Output {{API_KEY}}", "Write-Output ${env:API_KEY}", false},
		{"powershell double quote", shellPowerShell, `Write-Output "{{API_KEY}}"`, `Write-Output "${env:API_KEY}"`, false},
		{"powershell single quote", shellPowerShell, `Write-Output '{{API_KEY}}'`, "", true},
		{"powershell escaped quote", shellPowerShell, `Write-Output 'it''s {{API_KEY}}'`, "", true},
		{"cmd", shellCmd, `echo %PATH% {{API_KEY}}`, `echo %PATH% %API_KEY%`, false},
		{"unknown matching", shellUnknown, "run {{API_KEY}}", "", true},
		{"unknown non matching", shellUnknown, "run {{OTHER}}", "run {{OTHER}}", false},
		{"no tags", shellPOSIX, "ls -la", "ls -la", false},
		{"no closing", shellPOSIX, "echo {{API_KEY", "echo {{API_KEY", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := translateSecretTags(tc.kind, tc.in, keys)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTranslateNoTagsAllocs guards the no-tag path against regressing to an
// O(keys x length) scan: a command with no tags must not allocate per key.
func TestTranslateNoTagsAllocs(t *testing.T) {
	keys := manyKeys(100)
	cmd := strings.Repeat("echo hello ", 100)
	allocs := testing.AllocsPerRun(100, func() {
		if _, err := translateSecretTags(shellPOSIX, cmd, keys); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	if allocs > 1 {
		t.Fatalf("translating a tagless command allocated %.2f times, want <= 1", allocs)
	}
}

func manyKeys(n int) map[string]struct{} {
	out := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		out[fmt.Sprintf("KEY_%d", i)] = struct{}{}
	}
	return out
}
