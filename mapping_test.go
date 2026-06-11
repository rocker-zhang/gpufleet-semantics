package semantics

import "testing"

func TestResolveMappingDeterministic(t *testing.T) {
	edges := []DeviceJob{
		{Device: Device{UUID: "GPU-2"}, Job: Job{ID: "job-b"}},
		{Device: Device{UUID: "GPU-1"}, Job: Job{ID: "job-a"}},
		{Device: Device{UUID: "GPU-3"}, Job: Job{ID: "job-a"}},
		{Device: Device{UUID: "GPU-1"}, Job: Job{ID: "job-a"}}, // duplicate, deduped
	}
	got, err := ResolveMapping(edges)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 jobs, got %d", len(got))
	}
	// Jobs sorted by ID: job-a first.
	if got[0].Job.ID != "job-a" || got[1].Job.ID != "job-b" {
		t.Fatalf("jobs not sorted: %s, %s", got[0].Job.ID, got[1].Job.ID)
	}
	// job-a devices sorted by UUID, deduped: GPU-1, GPU-3.
	if len(got[0].Devices) != 2 ||
		got[0].Devices[0].UUID != "GPU-1" || got[0].Devices[1].UUID != "GPU-3" {
		t.Fatalf("job-a devices wrong: %+v", got[0].Devices)
	}
	if len(got[1].Devices) != 1 || got[1].Devices[0].UUID != "GPU-2" {
		t.Fatalf("job-b devices wrong: %+v", got[1].Devices)
	}
}

func TestResolveMappingConflict(t *testing.T) {
	edges := []DeviceJob{
		{Device: Device{UUID: "GPU-1"}, Job: Job{ID: "job-a"}},
		{Device: Device{UUID: "GPU-1"}, Job: Job{ID: "job-b"}}, // same device, two jobs
	}
	if _, err := ResolveMapping(edges); err != ErrConflictingMapping {
		t.Errorf("want ErrConflictingMapping, got %v", err)
	}
}

func TestResolveMappingEmpty(t *testing.T) {
	got, err := ResolveMapping(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty result, got %+v", got)
	}
}
