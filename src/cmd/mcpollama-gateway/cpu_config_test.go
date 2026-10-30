package main

import (
	"bytes"
	"testing"
	"time"
)

func TestCPUFlagsAndEnv(t *testing.T) {
	var buf bytes.Buffer
	c, err := parseConfig(nil, noEnv, &buf)
	if err != nil || c.ProcRoot != "/proc" || c.SysRoot != "/sys" || c.GPUInterval != time.Second || c.SampleStore != 5*time.Second {
		t.Fatalf("%v %+v", err, c)
	}
	c, err = parseConfig([]string{"--proc-root", "/fake/proc", "--sys-root", "/fake/sys", "--sample-store-interval", "10s"}, noEnv, &buf)
	if err != nil || c.ProcRoot != "/fake/proc" || c.SysRoot != "/fake/sys" || c.SampleStore != 10*time.Second {
		t.Fatalf("%v %+v", err, c)
	}
	env := func(k string) string {
		return map[string]string{"MCPGW_PROC_ROOT": "/e/proc", "MCPGW_SYS_ROOT": "/e/sys", "MCPGW_SAMPLE_STORE_INTERVAL": "30s"}[k]
	}
	c, err = parseConfig(nil, env, &buf)
	if err != nil || c.ProcRoot != "/e/proc" || c.SysRoot != "/e/sys" || c.SampleStore != 30*time.Second {
		t.Fatalf("%v %+v", err, c)
	}
	// the flag wins over the environment
	c, _ = parseConfig([]string{"--proc-root", "/flag"}, env, &buf)
	if c.ProcRoot != "/flag" {
		t.Fatalf("%+v", c)
	}
}

func TestSampleStoreIntervalValidation(t *testing.T) {
	var buf bytes.Buffer
	for _, tc := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"--gpu-interval", "1s", "--sample-store-interval", "5s"}, true},
		{[]string{"--gpu-interval", "5s", "--sample-store-interval", "5s"}, true},
		{[]string{"--gpu-interval", "5s", "--sample-store-interval", "1s"}, false},
		{[]string{"--gpu-interval", "0", "--sample-store-interval", "1s"}, true},
	} {
		c, err := parseConfig(tc.args, noEnv, &buf)
		if err == nil {
			err = c.validate()
		}
		if (err == nil) != tc.ok {
			t.Errorf("%v: err = %v", tc.args, err)
		}
	}
}
