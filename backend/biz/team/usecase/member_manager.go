package usecase

import (
	"context"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/errcode"
)

// MemberManager 实现 [domain.MemberManager]。
//
// 单实例部署只提供初始化管理员账号，不支持按邮箱邀请或新建团队成员，
// 因此三个邀请类操作显式返回不可用错误。缺少该实现会导致 DI 容器在
// Invoke 阶段 panic（biz/team/register.go 的 InvokeTeam 会强制解析该服务）。
type MemberManager struct{}

func NewMemberManager(i *do.Injector) (domain.MemberManager, error) {
	return &MemberManager{}, nil
}

func (m *MemberManager) AddUser(ctx context.Context, teamUser *domain.TeamUser, req *domain.AddTeamUserReq) (*domain.AddTeamUserResp, error) {
	return nil, errcode.ErrMemberInviteUnavailable
}

func (m *MemberManager) AddUserWithPassword(ctx context.Context, teamUser *domain.TeamUser, req *domain.AddTeamUserReq) (*domain.AddTeamUserWithPasswordResp, error) {
	return nil, errcode.ErrMemberInviteUnavailable
}

func (m *MemberManager) AddAdmin(ctx context.Context, teamUser *domain.TeamUser, req *domain.AddTeamAdminReq) (*domain.AddTeamAdminResp, error) {
	return nil, errcode.ErrMemberInviteUnavailable
}

func (m *MemberManager) AutoCreateOIDCMember(ctx context.Context, teamID uuid.UUID, external *domain.OIDCExternalUser) (*domain.User, error) {
	return nil, errcode.ErrMemberInviteUnavailable
}

var _ domain.MemberManager = (*MemberManager)(nil)
