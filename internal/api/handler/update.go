package handler

import (
	"dst-admin-go/internal/pkg/context"
	"dst-admin-go/internal/pkg/response"
	"dst-admin-go/internal/service/update"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UpdateHandler struct {
	updateService update.Update
}

func NewUpdateHandler(update update.Update) *UpdateHandler {
	return &UpdateHandler{
		updateService: update,
	}
}

func (h *UpdateHandler) RegisterRoute(router *gin.RouterGroup) {
	router.GET("/api/game/update", h.Update)
}

// Update 生成 swagger 文档注释
// @Summary 更新游戏
// @Description 更新游戏
// @Tags update
// @Success 200 {object} response.Response
// @Param isDelete query bool false "true 时清空游戏目录后全量重装（保留模组缓存）"
// @Router /api/game/update [get]
func (h *UpdateHandler) Update(ctx *gin.Context) {

	clusterName := context.GetClusterName(ctx)
	// 前端「删除并更新」传 isDelete=true：清空游戏目录后全量重装
	isDelete := ctx.Query("isDelete") == "true"

	err := h.updateService.Update(clusterName, isDelete)
	if err != nil {
		log.Panicln("更新游戏失败: ", err)
	}

	ctx.JSON(http.StatusOK, response.Response{
		Code: 200,
		Msg:  "update dst success",
		Data: nil,
	})
}
