package manifest

// Describe serializes a Manifest back to the v2 JSON structure. When withID
// is true, stage and pipeline IDs are included (for --inspect output);
// otherwise pipeline references use "name:" prefixes for readability.
func Describe(m *Manifest, withID bool) map[string]interface{} {
	runners := make(map[string]Runner)
	for i := range m.Pipelines {
		if m.Pipelines[i].Build != nil {
			runners[m.Pipelines[i].ID()] = m.Pipelines[i].Runner
		}
	}

	pls := make([]interface{}, 0, len(m.Pipelines))
	for i := range m.Pipelines {
		pls = append(pls, describePipeline(&m.Pipelines[i], m, runners, withID))
	}

	result := map[string]interface{}{
		"version":   "2",
		"pipelines": pls,
	}

	if len(m.Metadata) > 0 {
		result["metadata"] = m.Metadata
	}

	if len(m.Sources) > 0 {
		sources := make(map[string]interface{})
		for _, s := range m.Sources {
			sd := make(map[string]interface{})
			if len(s.Items) > 0 {
				sd["items"] = s.Items
			}
			if len(s.Options) > 0 {
				sd["options"] = s.Options
			}
			sources[s.Type] = sd
		}
		result["sources"] = sources
	}

	return result
}

func describePipeline(p *Pipeline, m *Manifest, runners map[string]Runner, withID bool) map[string]interface{} {
	desc := map[string]interface{}{
		"name": p.Name,
	}

	if p.Build != nil {
		if withID {
			desc["build"] = *p.Build
		} else {
			bp := m.PipelineByID(*p.Build)
			if bp != nil {
				desc["build"] = "name:" + bp.Name
			} else {
				desc["build"] = *p.Build
			}
		}
	}

	if r, ok := runners[p.ID()]; ok && r.Name != "" {
		desc["runner"] = r.Name
	}

	if p.SourceEpoch != nil {
		desc["source-epoch"] = *p.SourceEpoch
	}

	stages := make([]interface{}, 0, len(p.Stages))
	for i := range p.Stages {
		stages = append(stages, describeStage(&p.Stages[i], m, withID))
	}
	desc["stages"] = stages

	return desc
}

func describeStage(s *Stage, m *Manifest, withID bool) map[string]interface{} {
	desc := map[string]interface{}{
		"type": s.Type,
	}

	if withID {
		desc["id"] = s.ID()
	}

	if len(s.Options) > 0 {
		desc["options"] = s.Options
	}

	if s.Devices.Len() > 0 {
		devs := make(map[string]interface{})
		s.Devices.Range(func(name string, d *Device) bool {
			devs[name] = describeDevice(d)
			return true
		})
		desc["devices"] = devs
	}

	if s.Inputs.Len() > 0 {
		inputs := make(map[string]interface{})
		s.Inputs.Range(func(name string, ip *Input) bool {
			inputs[name] = describeInput(ip, m, withID)
			return true
		})
		desc["inputs"] = inputs
	}

	if s.Mounts.Len() > 0 {
		mounts := make([]interface{}, 0, s.Mounts.Len())
		s.Mounts.Range(func(_ string, mnt *Mount) bool {
			mounts = append(mounts, describeMount(mnt))
			return true
		})
		desc["mounts"] = mounts
	}

	return desc
}

func describeDevice(d *Device) map[string]interface{} {
	desc := map[string]interface{}{
		"type": d.Type,
	}
	if len(d.Options) > 0 {
		desc["options"] = d.Options
	}
	if d.Parent != nil {
		desc["parent"] = d.Parent.Name
	}
	return desc
}

func describeInput(ip *Input, m *Manifest, withID bool) map[string]interface{} {
	desc := map[string]interface{}{
		"type":   ip.Type,
		"origin": ip.Origin,
	}
	if len(ip.Options) > 0 {
		desc["options"] = ip.Options
	}

	if len(ip.Refs) > 0 {
		refs := make(map[string]interface{})
		for ref, opts := range ip.Refs {
			key := ref
			if ip.Origin == "org.osbuild.pipeline" && !withID {
				bp := m.PipelineByID(ref)
				if bp != nil {
					key = "name:" + bp.Name
				}
			}
			refs[key] = opts
		}
		desc["references"] = refs
	}

	return desc
}

func describeMount(mnt *Mount) map[string]interface{} {
	desc := map[string]interface{}{
		"name": mnt.Name,
		"type": mnt.Type,
	}
	if mnt.Target != "" {
		desc["target"] = mnt.Target
	}
	if mnt.Device != nil {
		desc["source"] = mnt.Device.Name
	}
	if len(mnt.Options) > 0 {
		desc["options"] = mnt.Options
	}
	if mnt.Partition != nil {
		desc["partition"] = *mnt.Partition
	}
	return desc
}
