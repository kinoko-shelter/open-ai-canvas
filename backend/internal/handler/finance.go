package handler

import (
	"net/http"
	"strconv"
	"time"

	"infinite-canvas/backend/internal/service"

	"github.com/gin-gonic/gin"
)

func RegisterFinanceRoutes(r *gin.RouterGroup, svc *service.Service) {
	r.GET("/wallet", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
		wallet, err := svc.Wallet(user, c.Query("type"), page, limit)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, wallet)
	})
	r.POST("/wallet/redeem", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if !enforceRateLimit(c, "redeem:"+user.ID, 10, time.Hour) {
			return
		}
		var req struct {
			Code string `json:"code"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		account, err := svc.RedeemCredits(user, req.Code, c.ClientIP())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"account": account})
	})
	r.POST("/wallet/checkin", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		account, granted, err := svc.CheckinCredits(user)
		if err != nil {
			failService(c, err)
			return
		}
		if !granted {
			fail(c, http.StatusConflict, service.BadAuthRequest("今天已经签到过了"))
			return
		}
		ok(c, gin.H{"account": account, "granted": true})
	})
	r.GET("/wallet/team-credit-recipients", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		users, err := svc.TeamCreditRecipients(user)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"users": users})
	})
	r.POST("/wallet/team-credit-transfers", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if !enforceRateLimit(c, "team-credit-transfer:"+user.ID, 60, time.Hour) {
			return
		}
		var req service.TeamCreditTransferRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.TransferTeamCredits(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})

	r.GET("/admin/settings/linuxdo", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		setting, err := svc.AdminLinuxDOSetting(user)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"setting": setting})
	})
	r.PATCH("/admin/settings/linuxdo", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.LinuxDOSettingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		setting, err := svc.UpdateLinuxDOSetting(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"setting": setting})
	})
	r.GET("/admin/settings/registration", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		setting, err := svc.AdminRegistrationSetting(user)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"setting": setting})
	})
	r.PATCH("/admin/settings/registration", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.RegistrationSettingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		setting, err := svc.UpdateRegistrationSetting(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"setting": setting})
	})
	r.GET("/admin/settings/credits", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		policy, err := svc.AdminCreditPolicy(user)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"policy": policy})
	})
	r.PATCH("/admin/settings/credits", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.CreditPolicy
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		policy, err := svc.UpdateCreditPolicy(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"policy": policy})
	})
	r.GET("/admin/settings/email", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		setting, err := svc.AdminEmailSetting(user)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"setting": setting})
	})
	r.PATCH("/admin/settings/email", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.EmailSettingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		setting, err := svc.UpdateEmailSetting(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"setting": setting})
	})

	r.GET("/admin/channels/:id/models", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		items, err := svc.AdminChannelModels(user, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"models": items})
	})
	r.POST("/admin/channels/:id/models/fetch", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if !enforceRateLimit(c, "admin-channel-models-fetch:"+user.ID+":"+c.Param("id"), 10, time.Minute) {
			return
		}
		// 上游密钥只在 service 内使用，handler 仅返回去重后的模型标识和新增数量。
		result, err := svc.FetchAdminChannelModels(c.Request.Context(), user, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	r.POST("/admin/channels/:id/models/test", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if !enforceRateLimit(c, "admin-channel-model-test:"+user.ID+":"+c.Param("id"), 5, time.Minute) {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		var req service.ChannelModelRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.TestAdminChannelModel(c.Request.Context(), user, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	r.POST("/admin/channels/:id/models", func(c *gin.Context) {
		saveChannelModel(c, svc, "")
	})
	r.PATCH("/admin/channels/:id/models/:modelId", func(c *gin.Context) {
		saveChannelModel(c, svc, c.Param("modelId"))
	})
	r.DELETE("/admin/channels/:id/models/:modelId", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.DeleteAdminChannelModel(user, c.Param("id"), c.Param("modelId")); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"ok": true})
	})

	r.GET("/admin/redeem-batches", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		items, err := svc.AdminRedeemBatchPage(user, service.AdminListQuery{Keyword: c.Query("keyword"), Status: c.Query("validity"), Page: page, Limit: limit})
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, items)
	})
	r.POST("/admin/redeem-batches", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if !enforceRateLimit(c, "redeem-batch-create:"+user.ID, 20, time.Hour) {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		var req service.CreateRedeemBatchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.AdminCreateRedeemBatch(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		ok(c, result)
	})
	r.GET("/admin/redeem-batches/:id/codes", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		result, err := svc.AdminRedeemCodePage(user, c.Param("id"), c.Query("status"), page, limit)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		ok(c, result)
	})
	r.POST("/admin/redeem-batches/:id/disable", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		count, err := svc.AdminDisableRedeemBatch(user, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"disabledCount": count})
	})
	r.POST("/admin/redeem-batches/:id/codes/:codeId/disable", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if err := svc.AdminDisableRedeemCode(user, c.Param("id"), c.Param("codeId")); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"ok": true})
	})
	r.POST("/admin/users/:id/credits/adjust", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.AdminCreditAdjustmentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		account, err := svc.AdminAdjustCredits(user, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"account": account})
	})
	r.GET("/admin/team-credit-management", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		result, err := svc.AdminTeamCreditOverview(user)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		ok(c, result)
	})
	r.POST("/admin/team-credit-transfers", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		if !enforceRateLimit(c, "admin-team-credit-transfer:"+user.ID, 60, time.Hour) {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		var req service.AdminTeamCreditTransferRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.AdminTransferTeamCredits(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		ok(c, result)
	})
	r.GET("/admin/settlement-statements", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		deptID, err := optionalInt64Query(c, "deptId")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		projectID, err := optionalInt64Query(c, "projectId")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.SettlementStatementPage(user, c.Query("month"), c.DefaultQuery("status", "all"), deptID, projectID, page, limit)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	r.GET("/admin/settlement-statements/references", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		result, err := svc.SettlementReferences(user)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	r.GET("/admin/settlement-statements/orders", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		deptID, err := optionalInt64Query(c, "deptId")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if deptID == nil {
			fail(c, http.StatusBadRequest, service.BadAuthRequest("请选择团队"))
			return
		}
		projectID, err := optionalInt64Query(c, "projectId")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.SettlementStatementOrders(user, c.Query("month"), *deptID, c.Query("userId"), projectID, c.Query("settlementType"), c.DefaultQuery("status", "all"), page, limit)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	r.POST("/admin/settlement-statements/confirm", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		var req service.ConfirmSettlementStatementsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		confirmed, err := svc.ConfirmSettlementStatements(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"confirmedCount": confirmed})
	})
	r.GET("/admin/billing-orders", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		items, err := svc.AdminBillingOrderPage(user, service.AdminListQuery{Keyword: c.Query("keyword"), Status: c.DefaultQuery("status", "review"), Page: page, Limit: limit})
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, items)
	})
	r.POST("/admin/billing-orders/batch-resolve", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		var req service.ResolveBillingBatchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		result, err := svc.ResolveBillingOrders(user, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	r.POST("/admin/billing-orders/:id/resolve", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.ResolveBillingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		order, err := svc.ResolveBillingOrder(user, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"order": order})
	})
}

func saveChannelModel(c *gin.Context, svc *service.Service, id string) {
	user, err := currentUser(c, svc)
	if err != nil {
		failService(c, err)
		return
	}
	var req service.ChannelModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	item, err := svc.SaveAdminChannelModel(user, c.Param("id"), id, req)
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, gin.H{"model": item})
}

func optionalInt64Query(c *gin.Context, key string) (*int64, error) {
	value := c.Query(key)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return nil, service.BadAuthRequest("筛选参数无效")
	}
	return &parsed, nil
}
