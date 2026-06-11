package manifest

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/supakeen/image-assembler/internal/jsonutil"
)

// ID computes a content-addressed SHA-256 hash from the stage's type,
// build/base refs, options, source epoch, inputs, and mounts. The hash
// uses jsonutil.Marshal (Python-compatible JSON) so IDs match Python
// osbuild's computation and cached objects are cross-compatible.
func (s *Stage) ID() string {
	h := sha256.New()
	write := func(v interface{}) {
		b, _ := jsonutil.Marshal(v)
		h.Write(b)
	}

	write(s.Name())
	write(s.Build)
	write(s.Base)
	write(s.Options)

	if s.SourceEpoch != nil {
		write(*s.SourceEpoch)
	}

	if s.Inputs.Len() > 0 {
		inputIDs := make(map[string]interface{})
		s.Inputs.Range(func(name string, ip *Input) bool {
			inputIDs[name] = ip.ID()
			return true
		})
		write(inputIDs)
	}

	if s.Mounts.Len() > 0 {
		mountIDs := make([]interface{}, 0, s.Mounts.Len())
		s.Mounts.Range(func(_ string, m *Mount) bool {
			mountIDs = append(mountIDs, m.ID())
			return true
		})
		write(mountIDs)
	}

	return hex.EncodeToString(h.Sum(nil))
}

func (ip *Input) ID() string {
	h := sha256.New()
	write := func(v interface{}) {
		b, _ := jsonutil.Marshal(v)
		h.Write(b)
	}

	write(ip.Type)
	write(ip.Origin)
	write(ip.Refs)
	write(ip.Options)

	return hex.EncodeToString(h.Sum(nil))
}

func (d *Device) ID() string {
	h := sha256.New()
	write := func(v interface{}) {
		b, _ := jsonutil.Marshal(v)
		h.Write(b)
	}

	write(d.Type)
	if d.Parent != nil {
		write(d.Parent.ID())
	}
	write(d.Options)

	return hex.EncodeToString(h.Sum(nil))
}

func (m *Mount) ID() string {
	h := sha256.New()
	write := func(v interface{}) {
		b, _ := jsonutil.Marshal(v)
		h.Write(b)
	}

	write(m.Type)
	if m.Device != nil {
		write(m.Device.ID())
	}
	if m.Partition != nil {
		write(*m.Partition)
	}
	if m.Target != "" {
		write(m.Target)
	}
	write(m.Options)

	return hex.EncodeToString(h.Sum(nil))
}

func (s *Source) ID() string {
	h := sha256.New()
	write := func(v interface{}) {
		b, _ := jsonutil.Marshal(v)
		h.Write(b)
	}

	write(s.Type)
	write(s.Items)

	return hex.EncodeToString(h.Sum(nil))
}
