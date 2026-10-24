package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/consts"
	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/pkg/taskflow"
)

func TestTeamHostListDegradesToOfflineWhenTaskflowUnreachable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	teamID := uuid.New()

	h := &TeamHostUsecase{
		repo:     &teamHostRepoStub{hosts: []*db.Host{hostWithVM("host-1", "vm-1")}},
		logger:   slog.Default(),
		taskflow: &teamTaskflowClientStub{err: errors.New("taskflow unavailable")},
	}

	got, err := h.List(ctx, &domain.TeamUser{
		User: &domain.User{ID: uuid.New()},
		Team: &domain.Team{ID: teamID},
	})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(got.Hosts) != 1 {
		t.Fatalf("List() returned %d hosts, want 1", len(got.Hosts))
	}

	host := got.Hosts[0]
	if host.ID != "host-1" {
		t.Fatalf("host id = %q, want %q", host.ID, "host-1")
	}
	if host.Status != consts.HostStatusOffline {
		t.Fatalf("host status = %q, want %q", host.Status, consts.HostStatusOffline)
	}
	if len(host.VirtualMachines) != 1 {
		t.Fatalf("host has %d vms, want 1", len(host.VirtualMachines))
	}
	if vmStatus := host.VirtualMachines[0].Status; vmStatus != taskflow.VirtualMachineStatusOffline {
		t.Fatalf("vm status = %q, want %q", vmStatus, taskflow.VirtualMachineStatusOffline)
	}
}

func hostWithVM(hostID, vmID string) *db.Host {
	return &db.Host{
		ID:       hostID,
		Hostname: hostID,
		Edges: db.HostEdges{
			Vms: []*db.VirtualMachine{{
				ID:         vmID,
				IsRecycled: false,
				CreatedAt:  time.Now().Add(-time.Hour),
			}},
		},
	}
}

type teamHostRepoStub struct {
	hosts []*db.Host
	seen  uuid.UUID
}

func (s *teamHostRepoStub) List(ctx context.Context, teamID uuid.UUID) ([]*db.Host, error) {
	s.seen = teamID
	return s.hosts, nil
}

func (s *teamHostRepoStub) Delete(context.Context, *domain.TeamUser, string) error {
	return nil
}

func (s *teamHostRepoStub) UpsertHost(context.Context, *domain.User, *taskflow.Host) error {
	return nil
}

func (s *teamHostRepoStub) Update(context.Context, *domain.TeamUser, *domain.UpdateTeamHostReq) (*db.Host, error) {
	return nil, nil
}

type teamTaskflowClientStub struct {
	err error
}

func (s *teamTaskflowClientStub) Host() taskflow.Hoster {
	return &teamHosterStub{err: s.err}
}

func (s *teamTaskflowClientStub) VirtualMachiner() taskflow.VirtualMachiner {
	return &teamVMStub{err: s.err}
}

func (s *teamTaskflowClientStub) FileManager() taskflow.FileManager { return nil }

func (s *teamTaskflowClientStub) TaskManager() taskflow.TaskManager { return nil }

func (s *teamTaskflowClientStub) PortForwarder() taskflow.PortForwarder { return nil }

func (s *teamTaskflowClientStub) Stats(context.Context) (*taskflow.Stats, error) {
	return nil, nil
}

func (s *teamTaskflowClientStub) TaskLive(context.Context, string, bool, func(*taskflow.TaskChunk) error) error {
	return nil
}

type teamHosterStub struct {
	err error
}

func (s *teamHosterStub) List(context.Context, string) (map[string]*taskflow.Host, error) {
	return nil, nil
}

func (s *teamHosterStub) IsOnline(context.Context, *taskflow.IsOnlineReq[string]) (*taskflow.IsOnlineResp, error) {
	return isOnlineResult(s.err)
}

type teamVMStub struct {
	err error
}

func (s *teamVMStub) Create(context.Context, *taskflow.CreateVirtualMachineReq) (*taskflow.VirtualMachine, error) {
	return nil, nil
}

func (s *teamVMStub) Delete(context.Context, *taskflow.DeleteVirtualMachineReq) error {
	return nil
}

func (s *teamVMStub) Hibernate(context.Context, *taskflow.HibernateVirtualMachineReq) error {
	return nil
}

func (s *teamVMStub) Resume(context.Context, *taskflow.ResumeVirtualMachineReq) error {
	return nil
}

func (s *teamVMStub) List(context.Context, string) ([]*taskflow.VirtualMachine, error) {
	return nil, nil
}

func (s *teamVMStub) Info(context.Context, taskflow.VirtualMachineInfoReq) (*taskflow.VirtualMachine, error) {
	return nil, nil
}

func (s *teamVMStub) Terminal(context.Context, *taskflow.TerminalReq) (taskflow.Sheller, error) {
	return nil, nil
}

func (s *teamVMStub) TerminalList(context.Context, string) ([]*taskflow.Terminal, error) {
	return nil, nil
}

func (s *teamVMStub) CloseTerminal(context.Context, *taskflow.CloseTerminalReq) error {
	return nil
}

func (s *teamVMStub) Reports(context.Context, taskflow.ReportSubscribeReq) (taskflow.Reporter, error) {
	return nil, nil
}

func (s *teamVMStub) IsOnline(context.Context, *taskflow.IsOnlineReq[string]) (*taskflow.IsOnlineResp, error) {
	return isOnlineResult(s.err)
}

func isOnlineResult(err error) (*taskflow.IsOnlineResp, error) {
	if err != nil {
		return nil, err
	}
	return &taskflow.IsOnlineResp{}, nil
}
