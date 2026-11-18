package main

import "testing"

func TestParseDeployArgs(t *testing.T) {
	cases := []struct {
		args []string
		want deployOpts
	}{
		{nil, deployOpts{repo: "."}},
		{[]string{"/r"}, deployOpts{repo: "/r"}},
		// `kube deploy --help` used to look up a target named "--help".
		{[]string{"--help"}, deployOpts{repo: ".", help: true}},
		{[]string{"-h"}, deployOpts{repo: ".", help: true}},
		{[]string{"--ref", "feature/x", "/r"}, deployOpts{repo: "/r", ref: "feature/x"}},
		{[]string{"/r", "--ref=021fb838"}, deployOpts{repo: "/r", ref: "021fb838"}},
	}
	for _, c := range cases {
		got, err := parseDeployArgs(c.args)
		if err != nil || got != c.want {
			t.Errorf("parseDeployArgs(%q) = %+v, %v; want %+v", c.args, got, err, c.want)
		}
	}
	for _, bad := range [][]string{{"--ref"}, {"--ref="}, {"--bogus"}, {"/a", "/b"}} {
		if _, err := parseDeployArgs(bad); err == nil {
			t.Errorf("parseDeployArgs(%q) should fail", bad)
		}
	}
}
