package scoperpc

import (
	"reflect"
	"testing"
	"time"

	anubisv1 "github.com/gsoultan/anubis/gen/go/anubis/v1"
	scopedomain "github.com/gsoultan/anubis/internal/scope/domain"
)

// A field added to SyncSource has to be carried in BOTH directions, and the
// failure mode when it is not is silence: interval_seconds shipped on every
// response and was dropped on the way in, so the console's Refresh control
// reported success and scheduled nothing. Nobody sees a mapping that forgets a
// field — there is no error, just a value that never arrives.
//
// These two tests are the cheapest thing that would have caught it.

func TestSyncSourceRecordCarriesEveryInboundField(t *testing.T) {
	in := &anubisv1.SyncSource{
		Id:              "01a02d06-0000-0000-0000-000000000001",
		Axis:            "org",
		Kind:            "http",
		Status:          "active",
		ConfigJson:      `{"url":"https://erp.example.com/units"}`,
		IntervalSeconds: 3600,
	}

	got := syncSourceRecord(in)

	if got.ID != in.Id {
		t.Errorf("ID = %q, want %q", got.ID, in.Id)
	}
	if got.Axis != in.Axis {
		t.Errorf("Axis = %q, want %q", got.Axis, in.Axis)
	}
	if got.Kind != in.Kind {
		t.Errorf("Kind = %q, want %q", got.Kind, in.Kind)
	}
	if got.Status != in.Status {
		t.Errorf("Status = %q, want %q", got.Status, in.Status)
	}
	if string(got.Config) != in.ConfigJson {
		t.Errorf("Config = %q, want %q", got.Config, in.ConfigJson)
	}
	// The one that was actually missing.
	if got.IntervalSeconds != in.IntervalSeconds {
		t.Errorf("IntervalSeconds = %d, want %d — a schedule the caller asked for "+
			"and the handler dropped is a control that lies", got.IntervalSeconds, in.IntervalSeconds)
	}
}

// A nil source is what an empty request carries. It must map to a zero record
// rather than panic: a malformed call is the caller's mistake to be told about,
// not the server's to crash on.
func TestSyncSourceRecordHandlesNil(t *testing.T) {
	if got := syncSourceRecord(nil); !reflect.DeepEqual(got, scopedomain.SyncSourceRecord{}) {
		t.Errorf("nil source mapped to %+v, want the zero record", got)
	}
}

func TestSyncSourceProtoCarriesEveryOutboundField(t *testing.T) {
	last := time.Unix(1_700_000_000, 0)
	next := time.Unix(1_700_003_600, 0)
	in := scopedomain.SyncSourceRecord{
		ID: "01a02d06-0000-0000-0000-000000000002", TenantID: "t", Axis: "customer",
		Kind: "db_query", Status: "active", Config: []byte(`{"dsn":"x"}`),
		LastRunAt: &last, IntervalSeconds: 300, NextRunAt: &next,
	}

	got := syncSourceProto(in)

	if got.Id != in.ID || got.Axis != in.Axis || got.Kind != in.Kind || got.Status != in.Status {
		t.Errorf("identity fields did not survive: %+v", got)
	}
	if got.ConfigJson != string(in.Config) {
		t.Errorf("ConfigJson = %q, want %q", got.ConfigJson, in.Config)
	}
	if got.IntervalSeconds != in.IntervalSeconds {
		t.Errorf("IntervalSeconds = %d, want %d", got.IntervalSeconds, in.IntervalSeconds)
	}
	if got.LastRunAt != last.Unix() {
		t.Errorf("LastRunAt = %d, want %d", got.LastRunAt, last.Unix())
	}
	if got.NextRunAt != next.Unix() {
		t.Errorf("NextRunAt = %d, want %d", got.NextRunAt, next.Unix())
	}
}

// A manual source carries neither timestamp, and zero is how the wire says so.
func TestSyncSourceProtoLeavesAbsentTimestampsZero(t *testing.T) {
	got := syncSourceProto(scopedomain.SyncSourceRecord{ID: "x", Axis: "org"})
	if got.LastRunAt != 0 || got.NextRunAt != 0 {
		t.Errorf("absent timestamps became %d/%d, want 0/0", got.LastRunAt, got.NextRunAt)
	}
}
