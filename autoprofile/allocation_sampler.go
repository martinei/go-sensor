package autoprofile

import (
	"bytes"
	"errors"
	"runtime/pprof"

	profile "github.com/instana/go-sensor/autoprofile/pprof/profile"
)

type allocationSampler struct{}

func newAllocationSampler() *allocationSampler {
	return &allocationSampler{}
}

func (as *allocationSampler) resetSampler() {
}

func (as *allocationSampler) startSampler() error {
	return nil
}

func (as *allocationSampler) stopSampler() error {
	return nil
}

func (as *allocationSampler) buildProfile(duration int64, timespan int64) (*Profile, error) {
	hp, err := as.readHeapProfile()
	if err != nil {
		return nil, err
	}

	if hp == nil {
		return nil, errors.New("no profile returned")
	}

	top, err := as.createAllocationCallGraph(hp)
	if err != nil {
		return nil, err
	}

	roots := make([]*callSite, 0)
	for _, child := range top.children {
		roots = append(roots, child)
	}

	return newProfile(categoryMemory, typeMemoryAllocation, unitByte, roots, duration, timespan), nil
}

func (as *allocationSampler) createAllocationCallGraph(p *profile.Profile) (*callSite, error) {
	// find "inuse_space" type index
	inuseSpaceTypeIndex := -1
	for i, s := range p.SampleType {
		if s.Type == "inuse_space" {
			inuseSpaceTypeIndex = i
			break
		}
	}

	// find "inuse_objects" type index
	inuseObjectsTypeIndex := -1
	for i, s := range p.SampleType {
		if s.Type == "inuse_objects" {
			inuseObjectsTypeIndex = i
			break
		}
	}

	if inuseSpaceTypeIndex == -1 || inuseObjectsTypeIndex == -1 {
		return nil, errors.New("unrecognized profile data")
	}

	// build call graph
	top := newCallSite("", "", 0)

	for _, s := range p.Sample {
		if shouldSkipStack(s) {
			continue
		}

		value := s.Value[inuseSpaceTypeIndex]
		if value == 0 {
			continue
		}

		count := s.Value[inuseObjectsTypeIndex]
		current := top
		for i := len(s.Location) - 1; i >= 0; i-- {
			l := s.Location[i]
			funcName, fileName, fileLine := readFuncInfo(l)

			if shouldSkipFrame(fileName, funcName) {
				continue
			}

			current = current.findOrAddChild(funcName, fileName, fileLine)
		}

		current.increment(float64(value), int64(count))
	}

	return top, nil
}

func (as *allocationSampler) readHeapProfile() (*profile.Profile, error) {
	buf := bytes.NewBuffer(nil)
	if err := pprof.WriteHeapProfile(buf); err != nil {
		return nil, err
	}

	p, err := profile.Parse(buf)
	if err != nil {
		return nil, err
	}

	if err := symbolizeProfile(p); err != nil {
		return nil, err
	}

	if err := p.CheckValid(); err != nil {
		return nil, err
	}

	return p, nil
}
