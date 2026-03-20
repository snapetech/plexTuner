package tuner

import "testing"

func TestNormalizeProfileName_HDHRStyleAliases(t *testing.T) {
	cases := map[string]string{
		"native":         profileDefault,
		"heavy":          profileDefault,
		"internet":       profileDashFast,
		"internet360":    profileAACCFR,
		"mobile":         profileLowBitrate,
		"cell":           profileLowBitrate,
		"Internet-1080":  profileDashFast,
		"INTERNET480":    profileAACCFR,
		"pms-xcode":      profilePMSXcode,
		"unknown-custom": profileDefault,
	}
	for in, want := range cases {
		if got := normalizeProfileName(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}
