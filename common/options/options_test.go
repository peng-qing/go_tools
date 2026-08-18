package options

import "testing"

type testConfig struct {
	name  string
	count int
}

func TestWrapperOptionsApply(t *testing.T) {
	cfg := testConfig{name: "before"}
	opts := []Option[testConfig]{
		WrapperOptions[testConfig](func(v *testConfig) { v.name = "after" }),
		WrapperOptions[testConfig](func(v *testConfig) { v.count += 2 }),
	}
	for _, opt := range opts {
		opt.Apply(&cfg)
	}
	expected := testConfig{name: "after", count: 2}
	t.Logf("actual: %+v", cfg)
	t.Logf("expected: %+v", expected)
	if cfg.name != expected.name || cfg.count != expected.count {
		t.Fatalf("Apply() produced %+v", cfg)
	}
}
