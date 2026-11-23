package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestObjectStoreConfigRedaction(t *testing.T) {
	c := ObjectStoreConfig{Endpoint: "10.8.0.5:9000", AccessKey: "AKIA-ACCESS-VALUE", SecretKey: "SECRET-VALUE-xyz", Bucket: "bkt", KeyNamespace: "prod", TimeoutSeconds: 5}
	for _, v := range []any{c, &c} {
		for _, f := range []string{"%v", "%+v", "%#v", "%s"} {
			out := fmt.Sprintf(f, v)
			if strings.Contains(out, "AKIA-ACCESS-VALUE") || strings.Contains(out, "SECRET-VALUE-xyz") {
				t.Errorf("%s of %T leaks a secret: %s", f, v, out)
			}
		}
	}
	// A parent struct printed with %+v goes through the same String method.
	out := fmt.Sprintf("%+v", struct{ O ObjectStoreConfig }{c})
	if strings.Contains(out, "SECRET-VALUE-xyz") || strings.Contains(out, "AKIA-ACCESS-VALUE") {
		t.Errorf("nested value leaks: %s", out)
	}
}

func TestObjectStoreConfigCheck(t *testing.T) {
	ok := ObjectStoreConfig{Endpoint: "minio.example.com:9000", AccessKey: "a", SecretKey: "s", Bucket: "b", KeyNamespace: "prod", UseSSL: true}
	if err := ok.Check(); err != nil {
		t.Fatalf("valid TLS config: %v", err)
	}
	plainWG := ok
	plainWG.UseSSL, plainWG.Endpoint = false, "10.8.0.9:9000"
	if err := plainWG.Check(); err != nil {
		t.Errorf("plaintext on WireGuard must be accepted: %v", err)
	}
	cases := map[string]func(c *ObjectStoreConfig){
		"unset":                func(c *ObjectStoreConfig) { *c = ObjectStoreConfig{} },
		"no secret":            func(c *ObjectStoreConfig) { c.SecretKey = "" },
		"no namespace":         func(c *ObjectStoreConfig) { c.KeyNamespace = "" },
		"namespace uppercase":  func(c *ObjectStoreConfig) { c.KeyNamespace = "Prod" },
		"namespace slash":      func(c *ObjectStoreConfig) { c.KeyNamespace = "a/b" },
		"namespace dash first": func(c *ObjectStoreConfig) { c.KeyNamespace = "-a" },
		"namespace too long":   func(c *ObjectStoreConfig) { c.KeyNamespace = strings.Repeat("a", 33) },
		"scheme in endpoint":   func(c *ObjectStoreConfig) { c.Endpoint = "https://minio:9000" },
		"plaintext public ip":  func(c *ObjectStoreConfig) { c.UseSSL, c.Endpoint = false, "8.8.8.8:9000" },
		"plaintext hostname":   func(c *ObjectStoreConfig) { c.UseSSL, c.Endpoint = false, "minio.local:9000" },
		"plaintext CGNAT ip":   func(c *ObjectStoreConfig) { c.UseSSL, c.Endpoint = false, "100.64.1.4:9000" },
	}
	for name, mut := range cases {
		c := ok
		mut(&c)
		err := c.Check()
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if strings.Contains(err.Error(), "minio.example.com") || strings.Contains(err.Error(), "8.8.8.8") {
			t.Errorf("%s: error leaks a value: %v", name, err)
		}
	}
}

func TestExecutorConfigCheck(t *testing.T) {
	for _, ok := range []string{"", "personal", "a", "team-2", "abcdefghijklmnopqrstu"} {
		if err := (ExecutorConfig{EnvPrefix: ok}).Check(); err != nil {
			t.Errorf("prefix %q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"Personal", "1abc", "-a", "a_b", "a b", "a/b", "../x", "abcdefghijklmnopqrstuv", "a.env"} {
		if err := (ExecutorConfig{EnvPrefix: bad}).Check(); err == nil {
			t.Errorf("prefix %q should be rejected", bad)
		}
	}
}
