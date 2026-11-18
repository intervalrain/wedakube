package deploy

import (
	"regexp"
	"strings"
)

var topLevelVersion = regexp.MustCompile(`(?m)^version:[^\n]*$`)

// withAppcfgVersion sets appcfg.yaml's top-level `version:` to tag, adding it when missing.
//
// helmster derives the chart appVersion and every `jobs.<name>.imageTag` from that field, so
// without it the pre-upgrade migrator Job pulls the committed (often non-existent) tag while
// only the Deployment gets the new image. CI does the same with
// `yq -i ".version = \"$IMAGE_TAG\""` right before `helmster gen`
// (the CI chart-generation stage).
func withAppcfgVersion(appcfg []byte, tag string) []byte {
	line := "version: " + tag
	if loc := topLevelVersion.FindIndex(appcfg); loc != nil {
		out := make([]byte, 0, len(appcfg)+len(tag))
		out = append(out, appcfg[:loc[0]]...)
		out = append(out, line...)
		return append(out, appcfg[loc[1]:]...)
	}
	return append([]byte(line+"\n"), appcfg...)
}

// redact replaces every non-empty secret in s with <redacted>.
func redact(s string, secrets ...string) string {
	for _, v := range secrets {
		if v != "" {
			s = strings.ReplaceAll(s, v, "<redacted>")
		}
	}
	return s
}
