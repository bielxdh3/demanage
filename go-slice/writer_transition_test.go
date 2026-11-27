package main

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestCustomTagTransitionFailsClosedAndDrainsExpress(t *testing.T) {
	gate := newSyntheticWriterGate(customTagsDomain, expressWriter)
	releaseExpress, err := gate.acquire(customTagsDomain, expressWriter)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseExpress()

	transitionDone := make(chan error, 1)
	go func() {
		transitionDone <- gate.transitionTo(customTagsDomain, goWriter)
	}()
	if !gate.waitUntilDraining(time.Second) {
		releaseExpress()
		t.Fatal("transition did not close admission")
	}
	for _, writer := range []writerIdentity{expressWriter, goWriter} {
		if _, err := gate.acquire(customTagsDomain, writer); !errors.Is(err, errWriterUnavailable) {
			t.Fatalf("writer %q acquired during drain: %v", writer, err)
		}
	}
	select {
	case err := <-transitionDone:
		t.Fatalf("transition completed before Express drained: %v", err)
	default:
	}

	releaseExpress()
	select {
	case err := <-transitionDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("transition did not finish after Express drained")
	}
	if got := gate.activeWriter(customTagsDomain); got != goWriter {
		t.Fatalf("active writer = %q, want %q", got, goWriter)
	}
	if _, err := gate.acquire(customTagsDomain, expressWriter); !errors.Is(err, errWriterUnavailable) {
		t.Fatalf("retired Express writer acquired after cutover: %v", err)
	}
	releaseGo, err := gate.acquire(customTagsDomain, goWriter)
	if err != nil {
		t.Fatalf("sole active Go writer was rejected: %v", err)
	}
	releaseGo()
}

func TestCustomTagWriterGateRejectsInvalidAndOverlappingTransitions(t *testing.T) {
	gate := newSyntheticWriterGate(customTagsDomain, expressWriter)
	if err := gate.transitionTo(customTagsDomain, writerIdentity("unknown")); !errors.Is(err, errInvalidTransition) {
		t.Fatalf("unknown writer transition error = %v", err)
	}
	if err := gate.transitionTo(customTagsDomain, expressWriter); !errors.Is(err, errInvalidTransition) {
		t.Fatalf("same-writer transition error = %v", err)
	}
	if err := gate.transitionTo("entries", goWriter); !errors.Is(err, errInvalidTransition) {
		t.Fatalf("cross-domain transition error = %v", err)
	}
	if _, err := gate.acquire("entries", expressWriter); !errors.Is(err, errWriterUnavailable) {
		t.Fatalf("cross-domain writer acquisition error = %v", err)
	}

	releaseExpress, err := gate.acquire(customTagsDomain, expressWriter)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseExpress()
	transitionDone := make(chan error, 1)
	go func() {
		transitionDone <- gate.transitionTo(customTagsDomain, goWriter)
	}()
	if !gate.waitUntilDraining(time.Second) {
		releaseExpress()
		t.Fatal("transition did not enter drain")
	}
	if err := gate.transitionTo(customTagsDomain, goWriter); !errors.Is(err, errTransitionInProgress) {
		t.Fatalf("overlapping transition error = %v", err)
	}
	releaseExpress()
	select {
	case err := <-transitionDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("transition did not finish after Express drained")
	}
}

func TestCustomTagRollbackDoesNotReplayUnknownOrCommittedMutations(t *testing.T) {
	gate := newSyntheticWriterGate(customTagsDomain, expressWriter)
	store := newSyntheticTagStore()
	seed := syntheticTagWrite{
		operationID: "express-seed-1",
		tag: syntheticCustomTag{
			ID: "tag-express", UserID: "owner-synthetic-a", Scope: "expense", Name: "Casa", Color: "#112233",
		},
		outcome:   outcomeCommitted,
	}
	if err := applySyntheticTagWrite(gate, store, customTagsDomain, expressWriter, seed); err != nil {
		t.Fatal(err)
	}
	if err := gate.transitionTo(customTagsDomain, goWriter); err != nil {
		t.Fatal(err)
	}

	committed := syntheticTagWrite{
		operationID: "go-committed-2",
		tag: syntheticCustomTag{
			ID: "tag-go-committed", UserID: "owner-synthetic-a", Scope: "expense", Name: "Transporte", Color: "#445566",
		},
		outcome:   outcomeCommitted,
	}
	if err := applySyntheticTagWrite(gate, store, customTagsDomain, goWriter, committed); err != nil {
		t.Fatal(err)
	}
	unknown := syntheticTagWrite{
		operationID: "go-unknown-3",
		tag: syntheticCustomTag{
			ID: "tag-go-unknown", UserID: "owner-synthetic-b", Scope: "income", Name: "Reembolso", Color: "#AABBCC",
		},
		outcome:   outcomeUnknown,
	}
	if err := applySyntheticTagWrite(gate, store, customTagsDomain, goWriter, unknown); !errors.Is(err, errSyntheticOutcomeUnknown) {
		t.Fatalf("unknown response error = %v", err)
	}

	want := []syntheticCustomTag{
		{ID: "tag-express", UserID: "owner-synthetic-a", Scope: "expense", Name: "Casa", Color: "#112233"},
		{ID: "tag-go-committed", UserID: "owner-synthetic-a", Scope: "expense", Name: "Transporte", Color: "#445566"},
		{ID: "tag-go-unknown", UserID: "owner-synthetic-b", Scope: "income", Name: "Reembolso", Color: "#AABBCC"},
	}
	if err := store.reconcile(want); err != nil {
		t.Fatalf("pre-rollback reconciliation: %v", err)
	}
	releaseGo, err := gate.acquire(customTagsDomain, goWriter)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGo()
	rollbackDone := make(chan error, 1)
	go func() {
		rollbackDone <- gate.transitionTo(customTagsDomain, expressWriter)
	}()
	if !gate.waitUntilDraining(time.Second) {
		t.Fatal("rollback did not close admission")
	}
	for _, writer := range []writerIdentity{expressWriter, goWriter} {
		if _, err := gate.acquire(customTagsDomain, writer); !errors.Is(err, errWriterUnavailable) {
			t.Fatalf("writer %q acquired during rollback drain: %v", writer, err)
		}
	}
	select {
	case err := <-rollbackDone:
		t.Fatalf("rollback completed before Go drained: %v", err)
	default:
	}
	releaseGo()
	select {
	case err := <-rollbackDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("rollback did not finish after Go drained")
	}
	if err := store.reconcile(want); err != nil {
		t.Fatalf("post-rollback reconciliation: %v", err)
	}
	for _, operationID := range []string{seed.operationID, committed.operationID, unknown.operationID} {
		if got := store.attempts(operationID); got != 1 {
			t.Errorf("operation %q attempts after rollback = %d, want 1", operationID, got)
		}
	}
	if _, err := gate.acquire(customTagsDomain, goWriter); !errors.Is(err, errWriterUnavailable) {
		t.Fatalf("retired Go writer acquired after rollback: %v", err)
	}
	if got := gate.activeWriter(customTagsDomain); got != expressWriter {
		t.Fatalf("active writer after rollback = %q, want %q", got, expressWriter)
	}
	releaseExpress, err := gate.acquire(customTagsDomain, expressWriter)
	if err != nil {
		t.Fatalf("Express was not restored as sole writer: %v", err)
	}
	releaseExpress()
}

type syntheticDomain string

type writerIdentity string

const (
	customTagsDomain syntheticDomain = "custom-tags"
	expressWriter    writerIdentity  = "express"
	goWriter         writerIdentity  = "go"
)

var (
	errWriterUnavailable       = errors.New("synthetic writer unavailable")
	errInvalidTransition       = errors.New("invalid synthetic writer transition")
	errTransitionInProgress    = errors.New("synthetic writer transition already in progress")
	errSyntheticOutcomeUnknown = errors.New("synthetic mutation outcome unknown")
)

// One gate instance models the sole active writer identity for one domain.
type syntheticWriterGate struct {
	mu       sync.Mutex
	changed  *sync.Cond
	domain   syntheticDomain
	active   writerIdentity
	draining bool
	inFlight int
}

func newSyntheticWriterGate(domain syntheticDomain, active writerIdentity) *syntheticWriterGate {
	gate := &syntheticWriterGate{domain: domain, active: active}
	gate.changed = sync.NewCond(&gate.mu)
	return gate
}

func (gate *syntheticWriterGate) acquire(domain syntheticDomain, writer writerIdentity) (func(), error) {
	gate.mu.Lock()
	if domain != gate.domain || !knownSyntheticWriter(writer) || gate.draining || writer != gate.active {
		gate.mu.Unlock()
		return nil, errWriterUnavailable
	}
	gate.inFlight++
	gate.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			gate.mu.Lock()
			gate.inFlight--
			gate.changed.Broadcast()
			gate.mu.Unlock()
		})
	}, nil
}

func (gate *syntheticWriterGate) transitionTo(domain syntheticDomain, target writerIdentity) error {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if domain != gate.domain || !knownSyntheticWriter(target) {
		return errInvalidTransition
	}
	if gate.draining {
		return errTransitionInProgress
	}
	if target == gate.active {
		return errInvalidTransition
	}

	gate.draining = true
	gate.changed.Broadcast()
	for gate.inFlight > 0 {
		gate.changed.Wait()
	}
	gate.active = target
	gate.draining = false
	gate.changed.Broadcast()
	return nil
}

func (gate *syntheticWriterGate) activeWriter(domain syntheticDomain) writerIdentity {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if domain != gate.domain {
		return ""
	}
	return gate.active
}

func (gate *syntheticWriterGate) waitUntilDraining(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		gate.mu.Lock()
		draining := gate.draining
		gate.mu.Unlock()
		if draining {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

func knownSyntheticWriter(writer writerIdentity) bool {
	return writer == expressWriter || writer == goWriter
}

type syntheticCustomTag struct {
	ID     string
	UserID string
	Scope  string
	Name   string
	Color  string
}

type syntheticOutcome uint8

const (
	outcomeCommitted syntheticOutcome = iota + 1
	outcomeUnknown
)

type syntheticTagWrite struct {
	operationID string
	tag         syntheticCustomTag
	outcome     syntheticOutcome
}

type syntheticTagStore struct {
	mu       sync.Mutex
	tags     map[string]syntheticCustomTag
	attempts map[string]int
}

func newSyntheticTagStore() *syntheticTagStore {
	return &syntheticTagStore{
		tags:     make(map[string]syntheticCustomTag),
		attempts: make(map[string]int),
	}
}

func applySyntheticTagWrite(
	gate *syntheticWriterGate,
	store *syntheticTagStore,
	domain syntheticDomain,
	writer writerIdentity,
	write syntheticTagWrite,
) error {
	release, err := gate.acquire(domain, writer)
	if err != nil {
		return err
	}
	defer release()
	if write.outcome != outcomeCommitted && write.outcome != outcomeUnknown {
		return fmt.Errorf("invalid synthetic outcome %d", write.outcome)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.attempts[write.operationID]++
	store.tags[write.tag.ID] = write.tag
	if write.outcome == outcomeUnknown {
		// The write landed, but its caller has no commit receipt to replay safely.
		return errSyntheticOutcomeUnknown
	}
	return nil
}

func (store *syntheticTagStore) attempts(operationID string) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.attempts[operationID]
}

func (store *syntheticTagStore) reconcile(want []syntheticCustomTag) error {
	store.mu.Lock()
	got := make([]syntheticCustomTag, 0, len(store.tags))
	for _, tag := range store.tags {
		got = append(got, tag)
	}
	store.mu.Unlock()

	expected := append([]syntheticCustomTag(nil), want...)
	sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })
	sort.Slice(expected, func(i, j int) bool { return expected[i].ID < expected[j].ID })
	if !reflect.DeepEqual(got, expected) {
		return fmt.Errorf("synthetic tags differ: got %#v, want %#v", got, expected)
	}
	return nil
}
