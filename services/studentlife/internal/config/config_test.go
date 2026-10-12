package config

import "testing"

func TestLoadRequiresEncryptionKeys(t *testing.T) {
	t.Setenv("STUDENTLIFE_ENC_KEY", "")
	t.Setenv("STUDENTLIFE_BLIND_KEY", "studentlife-test-blind-key")
	if _, err := Load(); err == nil {
		t.Fatal("empty STUDENTLIFE_ENC_KEY must fail")
	}
	t.Setenv("STUDENTLIFE_ENC_KEY", "c2hvcnQ=")
	if _, err := Load(); err == nil {
		t.Fatal("short STUDENTLIFE_ENC_KEY must fail")
	}
	t.Setenv("STUDENTLIFE_ENC_KEY", "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=")
	t.Setenv("STUDENTLIFE_BLIND_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("empty STUDENTLIFE_BLIND_KEY must fail")
	}
	t.Setenv("STUDENTLIFE_BLIND_KEY", "studentlife-test-blind-key")
	c, err := Load()
	if err != nil || len(c.EncKey) != 32 || c.Port != "8094" || c.OpsURL != "" {
		t.Fatalf("valid config: %+v %v", c, err)
	}
}
