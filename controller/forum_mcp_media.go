package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	"image/png"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"loginTest/api"
	"loginTest/common"
	"loginTest/domain"
	"loginTest/model"

	"github.com/google/uuid"
	"github.com/spf13/viper"
)

const forumMaxImageBytes = 8 << 20

var forumMediaRefPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type forumImageInput struct {
	IdempotencyKey string `json:"idempotency_key"`
	MIME           string `json:"mime" jsonschema:"image/png or image/jpeg"`
	SHA256         string `json:"sha256" jsonschema:"Lowercase SHA256 of decoded original image bytes"`
	DataBase64     string `json:"data_base64" jsonschema:"Raw base64, not a data URL; decoded size at most 8 MiB"`
}

func decodeForumImage(in forumImageInput) (image.Image, error) {
	if in.MIME != "image/png" && in.MIME != "image/jpeg" {
		return nil, domain.ErrBadRequest
	}
	if len(in.DataBase64) > base64.StdEncoding.EncodedLen(forumMaxImageBytes) {
		return nil, domain.ErrBadRequest
	}
	b, err := base64.StdEncoding.Strict().DecodeString(in.DataBase64)
	if err != nil || len(b) == 0 || len(b) > forumMaxImageBytes {
		return nil, domain.ErrBadRequest
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != in.SHA256 {
		return nil, domain.ErrBadRequest
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 || "image/"+format != in.MIME {
		return nil, domain.ErrBadRequest
	}
	im, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, domain.ErrBadRequest
	}
	return im, nil
}

func validForumImageURL(raw string) bool {
	base := viper.GetString("imagebed.public_base")
	if base == "" {
		base = "https://image.ssemarket.cn"
	}
	allowed, err := url.Parse(base)
	if err != nil {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && strings.EqualFold(u.Host, allowed.Host) && u.User == nil && u.Fragment == "" && !strings.ContainsAny(raw, "\r\n<> ()\\")
}

func forumUploadImage(ctx context.Context, in forumImageInput) (map[string]any, error) {
	p := forumActor(ctx)
	if p.User.BanEnd.After(time.Now()) {
		return nil, domain.ErrForbidden
	}
	im, err := decodeForumImage(in)
	if err != nil {
		return nil, err
	}
	// Hash bytes instead of storing the multi-megabyte input in the receipt.
	payload := struct{ Action, MIME, SHA256 string }{"upload_image", in.MIME, in.SHA256}
	return forumRunAction(ctx, in.IdempotencyKey, payload, func() (map[string]any, error) {
		if common.GetDB() == nil {
			return nil, domain.ErrInternal
		}
		f, err := os.CreateTemp("", "forum-agent-*.png")
		if err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())
		// Re-encoding strips metadata and trailing bytes; never trust a client
		// filename, SVG, HTML, or a remote URL supplied by a model.
		if err := png.Encode(f, im); err != nil {
			f.Close()
			return nil, err
		}
		info, err := f.Stat()
		f.Close()
		if err != nil || info.Size() > forumMaxImageBytes {
			return nil, domain.ErrBadRequest
		}
		imageURL, err := api.UploadImage("", f.Name())
		if err != nil || !validForumImageURL(imageURL) {
			return nil, domain.ErrInternal
		}
		media := model.ForumAgentMedia{MediaRef: uuid.NewString(), UserID: p.User.ID, SHA256: in.SHA256,
			URL: imageURL, Width: im.Bounds().Dx(), Height: im.Bounds().Dy()}
		if err := common.GetDB().WithContext(ctx).Create(&media).Error; err != nil {
			return nil, err
		}
		return map[string]any{"success": true, "status": "confirmed", "media_ref": media.MediaRef, "url": media.URL,
			"mime": "image/png", "width": media.Width, "height": media.Height}, nil
	})
}
