package service

import (
	"context"
	"errors"

	"loginTest/domain"
	"loginTest/repository"
)

type emojiService struct {
	emojis repository.EmojiRepository
}

func NewEmojiService(emojis repository.EmojiRepository) EmojiService {
	return &emojiService{emojis: emojis}
}

func validStickerItems(items []domain.EmojiStickerInput) error {
	if len(items) == 0 {
		return errors.New("没有收到表情图片")
	}
	if len(items) > domain.EmojiUploadMax {
		return errors.New("一次最多上传 20 张表情")
	}
	for _, item := range items {
		if item.URL == "" {
			return errors.New("表情图片地址无效")
		}
	}
	return nil
}

// GetPanel 一次拉齐表情面板：内置组（小汐/SSE娘，始终展示）+ 个人表情组（默认空）+ 我添加/自建的分享组。
func (s *emojiService) GetPanel(ctx context.Context, userID int) (*domain.EmojiPanel, error) {
	builtinPack, err := s.emojis.EnsureBuiltinPack(ctx)
	if err != nil {
		return nil, err
	}
	builtin, err := s.emojis.ListBuiltinStickers(ctx)
	if err != nil {
		return nil, err
	}
	// 启动种子没跑成（例如 DB 账号缺 DDL/INSERT 权限）时兜底补一次，幂等。
	if len(builtin) == 0 {
		if err := s.emojis.EnsureBuiltinStickers(ctx); err != nil {
			return nil, err
		}
		builtin, err = s.emojis.ListBuiltinStickers(ctx)
		if err != nil {
			return nil, err
		}
	}
	personalPack, personal, err := s.personalPackView(ctx, userID)
	if err != nil {
		return nil, err
	}
	packIDs, err := s.emojis.ListUserPackIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	packItems, err := s.emojis.ListPackStickers(ctx, packIDs)
	if err != nil {
		return nil, err
	}
	packs := make([]*domain.EmojiPack, 0, len(packIDs))
	for _, packID := range packIDs {
		pack, err := s.emojis.FindPack(ctx, packID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if pack.IsDeleted || pack.IsPersonal {
			continue
		}
		stickers := packItems[packID]
		pack.StickerCount = len(stickers)
		pack.AddedByMe = true
		pack.Preview = previewStickers(stickers)
		packs = append(packs, pack)
	}
	builtinPack.StickerCount = len(builtin)
	builtinPack.AddedByMe = true
	builtinPack.Preview = previewStickers(builtin)
	return &domain.EmojiPanel{
		BuiltinPack:  builtinPack,
		Builtin:      builtin,
		PersonalPack: personalPack,
		Personal:     personal,
		Packs:        packs,
		PackItems:    packItems,
	}, nil
}

// personalPackView 组装面板里的个人表情组：还没建过时给一个未落库的空组视图，
// 所以「我的表情」初始就是空的，不会有任何写库动作。
func (s *emojiService) personalPackView(ctx context.Context, userID int) (*domain.EmojiPack, []*domain.Emoji, error) {
	pack, err := s.emojis.FindPersonalPack(ctx, userID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, nil, err
		}
		return emptyPersonalPack(userID), []*domain.Emoji{}, nil
	}
	if pack.IsDeleted {
		return emptyPersonalPack(userID), []*domain.Emoji{}, nil
	}
	stickers, err := s.emojis.ListPersonalStickers(ctx, pack.ID)
	if err != nil {
		return nil, nil, err
	}
	pack.IsPersonal = true
	pack.StickerCount = len(stickers)
	pack.AddedByMe = true
	return pack, stickers, nil
}

func emptyPersonalPack(userID int) *domain.EmojiPack {
	return &domain.EmojiPack{
		Title:       domain.PersonalPackTitle,
		OwnerUserID: userID,
		IsPersonal:  true,
		AddedByMe:   true,
	}
}

// putPersonalStickers 把表情放进用户的个人表情组（isPersonal=true）。
// 按 URL 去重（同图重复上传 / 重复长按只入库一份），配额只对真正入库的张数生效。
func (s *emojiService) putPersonalStickers(ctx context.Context, userID int, items []domain.EmojiStickerInput) ([]*domain.Emoji, error) {
	if err := validStickerItems(items); err != nil {
		return nil, err
	}
	pack, err := s.emojis.EnsurePersonalPack(ctx, userID)
	if err != nil {
		return nil, err
	}
	fresh := make([]domain.EmojiStickerInput, 0, len(items))
	for i := range items {
		items[i].Name = domain.NormalizeEmojiName(items[i].Name)
		exists, err := s.emojis.ExistsStickerInPack(ctx, pack.ID, items[i].URL)
		if err != nil {
			return nil, err
		}
		if !exists {
			fresh = append(fresh, items[i])
		}
	}
	if len(fresh) == 0 {
		return []*domain.Emoji{}, nil
	}
	if err := s.checkQuota(ctx, userID, len(fresh)); err != nil {
		return nil, err
	}
	return s.emojis.CreateStickers(ctx, pack.ID, userID, fresh)
}

// UploadStickers 上传表情：进自己的个人表情组，不建分享组。
func (s *emojiService) UploadStickers(ctx context.Context, userID int, items []domain.EmojiStickerInput) ([]*domain.Emoji, error) {
	return s.putPersonalStickers(ctx, userID, items)
}

// CreatePack 上传一组表情并建组；isPublic 决定别人能否搜索并添加这组。
func (s *emojiService) CreatePack(ctx context.Context, userID int, title string, isPublic bool, items []domain.EmojiStickerInput) (*domain.EmojiPack, []*domain.Emoji, error) {
	if len(items) > domain.PackStickerMax {
		return nil, nil, errors.New("一个表情包组最多 30 张")
	}
	if err := validStickerItems(items); err != nil {
		return nil, nil, err
	}
	if err := s.checkQuota(ctx, userID, len(items)); err != nil {
		return nil, nil, err
	}
	for i := range items {
		items[i].Name = domain.NormalizeEmojiName(items[i].Name)
	}
	pack, err := s.emojis.CreatePack(ctx, domain.NormalizePackTitle(title), userID, isPublic)
	if err != nil {
		return nil, nil, err
	}
	created, err := s.emojis.CreateStickers(ctx, pack.ID, userID, items)
	if err != nil {
		return nil, nil, err
	}
	if len(created) > 0 {
		pack.CoverEmojiID = created[0].ID
	}
	pack.StickerCount = len(created)
	pack.AddedByMe = true
	pack.Preview = previewStickers(created)
	if err := s.emojis.AddUserPack(ctx, userID, pack.ID); err != nil {
		return nil, nil, err
	}
	return pack, created, nil
}

func (s *emojiService) checkQuota(ctx context.Context, userID, incoming int) error {
	count, err := s.emojis.CountOwnedStickers(ctx, userID)
	if err != nil {
		return err
	}
	if count+int64(incoming) > domain.UserEmojiMax {
		return errors.New("表情数量已达上限（300 张），请先删除一些")
	}
	return nil
}

// AddPack 把搜索到的组添加进自己的面板。只有公开组 / 内置组 / 自己的组能添加；
// 个人表情组不出现在搜索结果里，这里再挡一道。
func (s *emojiService) AddPack(ctx context.Context, userID, packID int) error {
	pack, err := s.emojis.FindPack(ctx, packID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errors.New("表情包组不存在")
		}
		return err
	}
	if pack.IsDeleted {
		return errors.New("表情包组已被删除")
	}
	if pack.IsPersonal {
		return errors.New("个人表情组不能添加")
	}
	if !pack.IsPublic && pack.OwnerUserID != userID {
		return errors.New("这个表情包组没有分享")
	}
	return s.emojis.AddUserPack(ctx, userID, packID)
}

func (s *emojiService) RemovePack(ctx context.Context, userID, packID int) error {
	return s.emojis.RemoveUserPack(ctx, userID, packID)
}

func (s *emojiService) DeleteSticker(ctx context.Context, userID, emojiID int) error {
	sticker, err := s.emojis.FindSticker(ctx, emojiID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errors.New("表情不存在")
		}
		return err
	}
	// 内置原件（ownerUserID=0）不能删；自己收藏进个人组的内置副本（isBuiltin=true,
	// ownerUserID=自己）只是移出自己的面板，历史消息不受影响（token 仍可解析）。
	if sticker.IsBuiltin && sticker.OwnerUserID == 0 {
		return errors.New("内置表情不能删除")
	}
	if sticker.OwnerUserID != userID {
		return errors.New("只能删除自己上传的表情")
	}
	return s.emojis.SoftDeleteSticker(ctx, emojiID)
}

func (s *emojiService) DeletePack(ctx context.Context, userID, packID int) error {
	pack, err := s.emojis.FindPack(ctx, packID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errors.New("表情包组不存在")
		}
		return err
	}
	if pack.IsPersonal {
		return errors.New("个人表情组不能删除，只能删除里面的表情")
	}
	if pack.OwnerUserID == 0 {
		return errors.New("内置表情包组不能删除")
	}
	if pack.OwnerUserID != userID {
		return errors.New("只能删除自己创建的表情包组")
	}
	return s.emojis.SoftDeletePack(ctx, packID)
}

// AddFromChat 长按聊天里发出来的表情 → 加进自己的个人表情组。
// 一律复制一份归自己（内置表情复制的是内置标记行，图片走前端内置资源）：
// 原作者之后删图不影响这边；已在个人组里的同图按 URL 去重、幂等返回已有行。
func (s *emojiService) AddFromChat(ctx context.Context, userID, emojiID int) (*domain.Emoji, error) {
	sticker, err := s.emojis.FindSticker(ctx, emojiID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errors.New("表情不存在")
		}
		return nil, err
	}
	// 非内置表情被作者删掉后，长按不再允许加入；内置资产永远存在，删除的是收藏行，不受影响。
	if sticker.IsDeleted && !sticker.IsBuiltin {
		return nil, errors.New("该表情已被作者删除")
	}
	created, err := s.putPersonalStickers(ctx, userID, []domain.EmojiStickerInput{{
		Name:      sticker.Name,
		URL:       sticker.URL,
		IsBuiltin: sticker.IsBuiltin,
	}})
	if err != nil {
		return nil, err
	}
	if len(created) == 0 {
		// 已在个人组里（按 URL 去重命中），把已存在的行给前端，方便面板定位。
		return s.findPersonalStickerByURL(ctx, userID, sticker.URL)
	}
	return created[0], nil
}

func (s *emojiService) findPersonalStickerByURL(ctx context.Context, userID int, url string) (*domain.Emoji, error) {
	pack, err := s.emojis.FindPersonalPack(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errors.New("添加表情失败，请重试")
		}
		return nil, err
	}
	stickers, err := s.emojis.ListPersonalStickers(ctx, pack.ID)
	if err != nil {
		return nil, err
	}
	for _, item := range stickers {
		if item.URL == url {
			return item, nil
		}
	}
	return nil, errors.New("添加表情失败，请重试")
}

func (s *emojiService) Search(ctx context.Context, userID int, keyword string) (*domain.EmojiSearchResult, error) {
	keyword = trimAndLimitKeyword(keyword)
	result := &domain.EmojiSearchResult{Stickers: []*domain.Emoji{}, Packs: []*domain.EmojiPack{}}
	if keyword == "" {
		return result, nil
	}
	stickers, err := s.emojis.SearchStickers(ctx, userID, keyword, domain.EmojiSearchLimit)
	if err != nil {
		return nil, err
	}
	packs, err := s.emojis.SearchPacks(ctx, userID, keyword, domain.EmojiSearchLimit)
	if err != nil {
		return nil, err
	}
	addedIDs, err := s.emojis.ListUserPackIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	added := make(map[int]bool, len(addedIDs))
	for _, id := range addedIDs {
		added[id] = true
	}
	packIDs := make([]int, 0, len(packs))
	for _, pack := range packs {
		packIDs = append(packIDs, pack.ID)
	}
	items, err := s.emojis.ListPackStickers(ctx, packIDs)
	if err != nil {
		return nil, err
	}
	for _, pack := range packs {
		count, err := s.emojis.CountPackStickers(ctx, pack.ID)
		if err != nil {
			return nil, err
		}
		pack.StickerCount = int(count)
		pack.AddedByMe = added[pack.ID]
		pack.Preview = previewStickers(items[pack.ID])
	}
	result.Stickers = stickers
	result.Packs = packs
	return result, nil
}

func trimAndLimitKeyword(keyword string) string {
	runes := []rune(keyword)
	if len(runes) > 50 {
		runes = runes[:50]
	}
	return string(runes)
}

// Resolve 按 token 里的 emojiID 批量取表情元数据，供前端渲染历史消息。
// 不过滤已删除的：软删除只让它们从面板消失，不让历史消息里的表情裂开。
func (s *emojiService) Resolve(ctx context.Context, ids []int) ([]*domain.Emoji, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 200 {
		ids = ids[:200]
	}
	return s.emojis.FindStickersByIDs(ctx, ids)
}

func previewStickers(stickers []*domain.Emoji) []*domain.Emoji {
	if len(stickers) <= domain.EmojiPackPreview {
		return stickers
	}
	return stickers[:domain.EmojiPackPreview]
}
