package controller

import (
	"errors"
	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/service"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type SueResponse struct {
	SueID        int
	Targettype   string
	Targettitle  string
	Targetdetail string
	Reason       string
	Time         time.Time
}

func GetSues(c *gin.Context) {
	items, err := service.Sues.ListPending(c.Request.Context())
	if err != nil {
		apperr.Write(c, err)
		return
	}
	out := make([]SueResponse, 0, len(items))
	for _, item := range items {
		out = append(out, SueResponse{
			SueID:        item.ID,
			Targettype:   item.TargetType,
			Targettitle:  item.TargetTitle,
			Targetdetail: item.TargetDetail,
			Reason:       item.Reason,
			Time:         item.Time,
		})
	}
	c.JSON(http.StatusOK, out)
}

type IDsue struct {
	SueID uint
}

func NoViolation(c *gin.Context) {
	var ID IDsue
	c.Bind(&ID)
	_ = service.Sues.MarkNoViolation(c.Request.Context(), int(ID.SueID))
}

func Violation(c *gin.Context) {
	var ID IDsue
	c.Bind(&ID)
	if err := service.Sues.MarkViolation(c.Request.Context(), int(ID.SueID)); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return
		}
		apperr.Write(c, err)
	}
}
