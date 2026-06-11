package sandbox

import "sort"

// DefaultCaps is the baseline set of Linux capabilities granted to every
// stage. Individual stages can request additional caps (e.g. CAP_NET_ADMIN)
// via their module metadata. This set matches Python osbuild's default.
var DefaultCaps = map[string]struct{}{
	"CAP_AUDIT_WRITE":      {},
	"CAP_CHOWN":            {},
	"CAP_DAC_OVERRIDE":     {},
	"CAP_DAC_READ_SEARCH":  {},
	"CAP_FOWNER":           {},
	"CAP_FSETID":           {},
	"CAP_IPC_LOCK":         {},
	"CAP_LINUX_IMMUTABLE":  {},
	"CAP_MAC_ADMIN":        {},
	"CAP_MAC_OVERRIDE":     {},
	"CAP_MKNOD":            {},
	"CAP_NET_RAW":          {},
	"CAP_SETFCAP":          {},
	"CAP_SETGID":           {},
	"CAP_SETPCAP":          {},
	"CAP_SETUID":           {},
	"CAP_SYS_ADMIN":        {},
	"CAP_SYS_CHROOT":       {},
	"CAP_SYS_NICE":         {},
}

func BuildCapArgs(caps map[string]struct{}) []string {
	if caps == nil {
		return nil
	}

	sorted := make([]string, 0, len(caps))
	for c := range caps {
		sorted = append(sorted, c)
	}
	sort.Strings(sorted)

	args := []string{"--cap-drop", "ALL"}
	for _, c := range sorted {
		args = append(args, "--cap-add", c)
	}
	return args
}
