package semantics

import (
	"errors"
	"sort"
)

// ----------------------------------------------------------------------------
// device -> job mapping (TASK-0006).
//
// The agent collects, for a window, which job owned which device. This library
// turns a flat list of (device, job) edges into a deterministic, job-grouped
// view that the cost/efficiency math consumes. It does NO collection: the edges
// are handed in already resolved. Output ordering is fully deterministic (jobs
// by ID, devices by UUID) so attribution is byte-for-byte reproducible.
// ----------------------------------------------------------------------------

// ErrConflictingMapping is returned when the same device UUID is mapped to two
// different jobs within one window — an ambiguous ownership the caller must
// resolve upstream (a device runs at most one job per window).
var ErrConflictingMapping = errors.New("semantics: device mapped to conflicting jobs in one window")

// JobDevices is one job and the devices it owned in a window, sorted by UUID.
type JobDevices struct {
	Job     Job
	Devices []Device
}

// ResolveMapping groups (device -> job) edges by job, deterministically. A
// device appearing twice under the SAME job is de-duplicated; a device mapped
// to two DIFFERENT jobs returns ErrConflictingMapping (a device owns at most
// one job per window). Output is sorted by job ID, devices by UUID.
func ResolveMapping(edges []DeviceJob) ([]JobDevices, error) {
	byJob := make(map[string]*JobDevices)
	jobOf := make(map[string]string) // device UUID -> job ID, to catch conflicts
	seen := make(map[string]map[string]bool)

	for _, e := range edges {
		duuid := e.Device.UUID
		if prev, ok := jobOf[duuid]; ok && prev != e.Job.ID {
			return nil, ErrConflictingMapping
		}
		jobOf[duuid] = e.Job.ID

		jd, ok := byJob[e.Job.ID]
		if !ok {
			jd = &JobDevices{Job: e.Job}
			byJob[e.Job.ID] = jd
			seen[e.Job.ID] = make(map[string]bool)
		}
		if !seen[e.Job.ID][duuid] {
			seen[e.Job.ID][duuid] = true
			jd.Devices = append(jd.Devices, e.Device)
		}
	}

	out := make([]JobDevices, 0, len(byJob))
	for _, jd := range byJob {
		sort.Slice(jd.Devices, func(i, j int) bool {
			return jd.Devices[i].UUID < jd.Devices[j].UUID
		})
		out = append(out, *jd)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Job.ID < out[j].Job.ID })
	return out, nil
}
