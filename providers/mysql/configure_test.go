package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

// configMap builds an *OrderedMap of string values from key-value pairs.
func configMap(kvs ...string) *provider.OrderedMap {
	m := provider.NewOrderedMap()
	for i := 0; i < len(kvs); i += 2 {
		m.Set(kvs[i], provider.StringVal(kvs[i+1]))
	}
	return m
}

func TestConfigure(t *testing.T) {
	cases := []struct {
		name      string
		config    *provider.OrderedMap
		errSubstr string
	}{
		{
			name:      "nil config",
			config:    nil,
			errSubstr: "requires configuration",
		},
		{
			name:      "missing endpoint",
			config:    configMap("auth", "password", "username", "u", "password", "p"),
			errSubstr: `"endpoint" is required`,
		},
		{
			name:      "missing auth",
			config:    configMap("endpoint", "localhost:3306", "username", "u", "password", "p"),
			errSubstr: `"auth" is required`,
		},
		{
			name:      "unknown auth mode",
			config:    configMap("endpoint", "localhost:3306", "auth", "ldap", "username", "u", "password", "p"),
			errSubstr: `auth must be "password" or "rds_iam"`,
		},
		{
			name:      "password mode missing username",
			config:    configMap("endpoint", "localhost:3306", "auth", "password", "password", "p"),
			errSubstr: `"username" is required`,
		},
		{
			name:      "password mode missing password",
			config:    configMap("endpoint", "localhost:3306", "auth", "password", "username", "u"),
			errSubstr: `"password" is required`,
		},
		{
			name: "tls mode invalid",
			config: configMap(
				"endpoint", "localhost:3306",
				"auth", "password",
				"username", "u",
				"password", "p",
				"tls", "bogus",
			),
			errSubstr: `"tls" must be "required", "skip-verify", or "disabled"`,
		},
	}

	f, _ := provider.Lookup("mysql")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := f()
			diags := p.Configure(context.Background(), tc.config)
			if !diags.HasErrors() {
				t.Fatalf("expected an error diagnostic, got none")
			}
			found := false
			for _, d := range diags {
				if strings.Contains(d.Message, tc.errSubstr) {
					found = true
					break
				}
			}
			if !found {
				msgs := make([]string, 0, len(diags))
				for _, d := range diags {
					msgs = append(msgs, d.Message)
				}
				t.Errorf("expected diagnostic containing %q, got: %v", tc.errSubstr, msgs)
			}
		})
	}
}
