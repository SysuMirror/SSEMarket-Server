package service

import (
	"context"
	"log"
	"strings"

	"loginTest/domain"
	"loginTest/repository"
	"loginTest/util"
)

// StaffService 接口定义见 interfaces.go。

type staffService struct {
	staff repository.StaffRepository
}

func NewStaffService(staff repository.StaffRepository) StaffService {
	return &staffService{staff: staff}
}

func (s *staffService) publicOf(a *domain.StaffAlias) (domain.StaffAliasPublic, bool) {
	if a == nil || a.AliasUserID <= 0 {
		return domain.StaffAliasPublic{}, false
	}
	return domain.StaffAliasPublic{
		AliasID: a.ID,
		Alias:   a.Alias,
		Title:   a.Title,
		UserID:  a.AliasUserID,
	}, true
}

// ensureShells 为尚未派生小号的职位行补建 users 行（管理员插行后无需重启即可生效）。
// 与 common.SyncStaffAliases（启动同步）同一套口径：昵称冲突的行拒绝派生并打日志。
func (s *staffService) ensureShells(ctx context.Context, aliases []*domain.StaffAlias) {
	for _, a := range aliases {
		if a == nil || a.AliasUserID > 0 {
			continue
		}
		if err := domain.ValidateStaffAliasRow(a.Alias, a.Title, a.StaffUserID); err != nil {
			log.Printf("[staff] 跳过非法职位行 id=%d: %v", a.ID, err)
			continue
		}
		email := domain.StaffAliasShellEmail(a.ID)
		conflict, err := s.staff.UsersByNameExcludeEmail(ctx, a.Alias, email)
		if err != nil {
			log.Printf("[staff] 查重失败 id=%d alias=%q: %v", a.ID, a.Alias, err)
			continue
		}
		if conflict > 0 {
			log.Printf("[staff] alias=%q 与已有用户昵称冲突，未派生职位小号（请管理员换个名字）", a.Alias)
			continue
		}
		uid, err := s.staff.CreateShellUser(ctx, a.ID, a.Alias, a.Title)
		if err != nil {
			// 并发下另一请求刚建好（email 唯一索引挡下）：回读兜底
			shell, shellErr := s.staff.FindShellUserByEmail(ctx, email)
			if shellErr != nil || shell == nil {
				log.Printf("[staff] 派生职位小号失败 id=%d alias=%q: %v", a.ID, a.Alias, err)
				continue
			}
			uid = shell.ID
		}
		if err := s.staff.UpdateAliasUserID(ctx, a.ID, uid); err != nil {
			log.Printf("[staff] 回写 alias_user_id 失败 id=%d: %v", a.ID, err)
			continue
		}
		a.AliasUserID = uid
	}
}

// threadContent 拼出线程全文（帖子正文 + 一级评论 + 楼中楼），@ 解析口径与 notifyMentions 一致。
func (s *staffService) threadContent(ctx context.Context, postID int) string {
	var sb strings.Builder
	if content, err := s.staff.FindPostContentByID(ctx, postID); err == nil {
		sb.WriteString(content)
		sb.WriteByte('\n')
	}
	if pcs, err := s.staff.ListPcommentContentsByPostID(ctx, postID); err == nil {
		for _, c := range pcs {
			sb.WriteString(c)
			sb.WriteByte('\n')
		}
	}
	if ccs, err := s.staff.ListCcommentContentsByPostID(ctx, postID); err == nil {
		for _, c := range ccs {
			sb.WriteString(c)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func (s *staffService) ListPublicAliases(ctx context.Context) ([]domain.StaffAliasPublic, error) {
	aliases, err := s.staff.ListAliases(ctx)
	if err != nil {
		return nil, err
	}
	if len(aliases) == 0 {
		return []domain.StaffAliasPublic{}, nil
	}
	s.ensureShells(ctx, aliases)
	out := make([]domain.StaffAliasPublic, 0, len(aliases))
	for _, a := range aliases {
		if p, ok := s.publicOf(a); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *staffService) ThreadAliasesForUser(ctx context.Context, userID, postID int) ([]domain.StaffAliasPublic, error) {
	if userID <= 0 || postID <= 0 {
		return []domain.StaffAliasPublic{}, nil
	}
	aliases, err := s.staff.FindAliasesByStaffUserID(ctx, userID)
	if err != nil || len(aliases) == 0 {
		// 普通用户（无职位行）到这里就返回，不碰线程扫描
		return []domain.StaffAliasPublic{}, nil
	}
	s.ensureShells(ctx, aliases)
	matched := domain.StaffAliasesMentioned(util.ParseMentionTokens(s.threadContent(ctx, postID)), aliases)
	out := make([]domain.StaffAliasPublic, 0, len(matched))
	for _, a := range matched {
		if p, ok := s.publicOf(a); ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *staffService) ShellsForNames(ctx context.Context, names []string) (map[string]*domain.User, error) {
	if len(names) == 0 {
		return nil, nil
	}
	aliases, err := s.staff.FindAliasesByNames(ctx, names)
	if err != nil || len(aliases) == 0 {
		return nil, err
	}
	s.ensureShells(ctx, aliases)
	nameSet := make(map[string]struct{}, len(names))
	for _, n := range names {
		nameSet[n] = struct{}{}
	}
	out := make(map[string]*domain.User, len(aliases))
	for _, a := range aliases {
		if a == nil || a.AliasUserID <= 0 {
			continue
		}
		if _, ok := nameSet[a.Alias]; !ok {
			continue
		}
		shell, err := s.staff.FindShellUserByEmail(ctx, domain.StaffAliasShellEmail(a.ID))
		if err != nil || shell == nil {
			continue
		}
		out[a.Alias] = shell
	}
	return out, nil
}

func (s *staffService) MatchedAliasesForTokens(ctx context.Context, tokens []string) ([]*domain.StaffAlias, error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	aliases, err := s.staff.FindAliasesByNames(ctx, tokens)
	if err != nil || len(aliases) == 0 {
		return nil, err
	}
	s.ensureShells(ctx, aliases)
	return domain.StaffAliasesMentioned(tokens, aliases), nil
}

func (s *staffService) AuthorizeAliasReply(ctx context.Context, userID, postID, asUserID int) (*domain.StaffAlias, error) {
	if userID <= 0 || postID <= 0 || asUserID <= 0 || asUserID == userID {
		return nil, domain.ErrForbidden
	}
	aliases, err := s.staff.FindAliasesByStaffUserID(ctx, userID)
	if err != nil || len(aliases) == 0 {
		return nil, domain.ErrForbidden
	}
	s.ensureShells(ctx, aliases)
	var target *domain.StaffAlias
	for _, a := range aliases {
		if a != nil && a.AliasUserID == asUserID {
			target = a
			break
		}
	}
	if target == nil {
		return nil, domain.ErrForbidden
	}
	// 只有这条帖子的线程里确实 @ 过该职位，才允许以职位身份回复
	if len(domain.StaffAliasesMentioned(util.ParseMentionTokens(s.threadContent(ctx, postID)), []*domain.StaffAlias{target})) == 0 {
		return nil, domain.ErrForbidden
	}
	return target, nil
}

// AliasShellUserIDsForUser 该真实账号名下的职位小号集合（staff_user_id 索引一次查询）。
// 普通用户没有职位行 → 返回空 map，评论列表的判定与改动前完全一致。
func (s *staffService) AliasShellUserIDsForUser(ctx context.Context, userID int) (map[int]struct{}, error) {
	if userID <= 0 {
		return nil, nil
	}
	aliases, err := s.staff.FindAliasesByStaffUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(aliases) == 0 {
		return nil, nil
	}
	s.ensureShells(ctx, aliases)
	out := make(map[int]struct{}, len(aliases))
	for _, a := range aliases {
		if a != nil && a.AliasUserID > 0 {
			out[a.AliasUserID] = struct{}{}
		}
	}
	return out, nil
}
