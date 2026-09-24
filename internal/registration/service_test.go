package registration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Emmanuelzyronis/forge/internal/domain"
	"github.com/Emmanuelzyronis/forge/internal/registration"
	"github.com/Emmanuelzyronis/forge/internal/store"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type memWorkerStore struct {
	mu     sync.Mutex
	byID   map[uuid.UUID]*domain.Worker
	byName map[string]*domain.Worker
}

func newMemWorkerStore() *memWorkerStore {
	return &memWorkerStore{
		byID:   make(map[uuid.UUID]*domain.Worker),
		byName: make(map[string]*domain.Worker),
	}
}

func (m *memWorkerStore) Upsert(_ context.Context, w *domain.Worker) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *w
	if existing, ok := m.byName[w.Name]; ok {
		// Keep the stable existing ID (mirrors the postgres RETURNING id behaviour).
		cp.ID = existing.ID
		w.ID = existing.ID
	}
	m.byID[cp.ID] = &cp
	m.byName[cp.Name] = &cp
	return nil
}

func (m *memWorkerStore) GetByID(_ context.Context, id uuid.UUID) (*domain.Worker, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *w
	return &cp, nil
}

func (m *memWorkerStore) Heartbeat(_ context.Context, workerID uuid.UUID, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[workerID]
	if !ok {
		return errors.New("not found")
	}
	w.LastHeartbeatAt = now
	return nil
}

func (m *memWorkerStore) MarkStale(_ context.Context, workerID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[workerID]
	if !ok {
		return errors.New("not found")
	}
	w.State = domain.WorkerStateStale
	return nil
}

func (m *memWorkerStore) MarkOffline(_ context.Context, workerID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[workerID]
	if !ok {
		return errors.New("not found")
	}
	w.State = domain.WorkerStateOffline
	return nil
}

func (m *memWorkerStore) ListActive(_ context.Context) ([]*domain.Worker, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var workers []*domain.Worker
	for _, w := range m.byID {
		if w.State != domain.WorkerStateStale && w.State != domain.WorkerStateOffline {
			cp := *w
			workers = append(workers, &cp)
		}
	}
	return workers, nil
}

var _ store.WorkerStore = (*memWorkerStore)(nil)

func newSvc(st *memWorkerStore) *registration.Service {
	return registration.NewService(st, zerolog.Nop())
}

func TestRegisterCreatesWorker(t *testing.T) {
	st := newMemWorkerStore()
	svc := newSvc(st)

	w, err := svc.Register(context.Background(), registration.RegisterRequest{
		Name:     "worker-1",
		Hostname: "host.local",
		PID:      1234,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if w.Name != "worker-1" {
		t.Errorf("Name: want worker-1, got %s", w.Name)
	}
	if w.State != domain.WorkerStateIdle {
		t.Errorf("State: want IDLE, got %s", w.State)
	}
	if w.ID == (uuid.UUID{}) {
		t.Error("ID should not be zero")
	}
}

func TestRegisterIdempotentByName(t *testing.T) {
	st := newMemWorkerStore()
	svc := newSvc(st)

	first, err := svc.Register(context.Background(), registration.RegisterRequest{
		Name: "worker-1", Hostname: "host-a", PID: 100,
	})
	if err != nil {
		t.Fatalf("first Register: %v", err)
	}

	second, err := svc.Register(context.Background(), registration.RegisterRequest{
		Name: "worker-1", Hostname: "host-b", PID: 200,
	})
	if err != nil {
		t.Fatalf("second Register: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("expected same ID on re-registration: %s vs %s", first.ID, second.ID)
	}
}

func TestHeartbeatUpdatesTimestamp(t *testing.T) {
	st := newMemWorkerStore()
	svc := newSvc(st)

	w, err := svc.Register(context.Background(), registration.RegisterRequest{
		Name: "worker-1", Hostname: "host.local", PID: 1,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	before := w.LastHeartbeatAt
	time.Sleep(2 * time.Millisecond)

	if err := svc.Heartbeat(context.Background(), w.ID); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	stored, _ := st.GetByID(context.Background(), w.ID)
	if !stored.LastHeartbeatAt.After(before) {
		t.Error("LastHeartbeatAt not updated by heartbeat")
	}
}

func TestOfflineMarksWorker(t *testing.T) {
	st := newMemWorkerStore()
	svc := newSvc(st)

	w, err := svc.Register(context.Background(), registration.RegisterRequest{
		Name: "worker-1", Hostname: "host.local", PID: 1,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.Offline(context.Background(), w.ID); err != nil {
		t.Fatalf("Offline: %v", err)
	}

	stored, _ := st.GetByID(context.Background(), w.ID)
	if stored.State != domain.WorkerStateOffline {
		t.Errorf("State: want OFFLINE, got %s", stored.State)
	}
}

func TestListActiveExcludesOfflineAndStale(t *testing.T) {
	st := newMemWorkerStore()
	svc := newSvc(st)

	for _, name := range []string{"w1", "w2", "w3"} {
		if _, err := svc.Register(context.Background(), registration.RegisterRequest{Name: name, PID: 1}); err != nil {
			t.Fatalf("Register %s: %v", name, err)
		}
	}

	workers, _ := st.ListActive(context.Background())
	for _, w := range workers {
		if w.Name == "w1" {
			st.MarkOffline(context.Background(), w.ID) //nolint:errcheck
		}
		if w.Name == "w2" {
			st.MarkStale(context.Background(), w.ID) //nolint:errcheck
		}
	}

	active, err := st.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("want 1 active worker, got %d", len(active))
	}
	if active[0].Name != "w3" {
		t.Errorf("want w3, got %s", active[0].Name)
	}
}
