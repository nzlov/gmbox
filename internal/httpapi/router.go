package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	stdmail "net/mail"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gmbox/internal/auth"
	"gmbox/internal/mail"
	"gmbox/internal/model"
	"gmbox/internal/runtime"
)

// NewRouter 组装 API、鉴权中间件和前端静态资源路由。
func NewRouter(app *runtime.App, assets fs.FS) *gin.Engine {
	router := gin.New()
	router.Use(slogRequestLogger(app))
	router.Use(gin.Recovery())

	api := router.Group("/api")
	registerPublic(api, app)
	registerProtected(api, app)

	registerFrontend(router, assets)
	return router
}

// slogRequestLogger 使用统一结构化日志记录 HTTP 请求，便于与后端任务日志按等级集中排查。
func slogRequestLogger(app *runtime.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"client_ip", c.ClientIP(),
		}
		if app != nil && app.Config != nil && app.Config.DebugMode() {
			attrs = append(attrs, "query", redactQueryForLog(c.Request.URL), "user_agent", c.Request.UserAgent())
		}
		if c.Writer.Status() >= http.StatusInternalServerError {
			slog.Error("HTTP 请求完成", attrs...)
			return
		}
		if c.Writer.Status() >= http.StatusBadRequest {
			slog.Warn("HTTP 请求完成", attrs...)
			return
		}
		slog.Info("HTTP 请求完成", attrs...)
	}
}

// redactQueryForLog 脱敏 URL 查询参数中的一次性凭证，避免调试日志泄漏 OAuth 敏感信息。
func redactQueryForLog(target *url.URL) string {
	if target == nil {
		return ""
	}
	query := target.Query()
	for _, key := range []string{"code", "state", "access_token", "refresh_token", "code_verifier"} {
		if _, ok := query[key]; ok {
			query.Set(key, "<redacted>")
		}
	}
	return query.Encode()
}

// registerPublic 注册登录等无需鉴权的接口。
func registerPublic(api *gin.RouterGroup, app *runtime.App) {
	authGroup := api.Group("/auth")
	authGroup.POST("/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		var user model.User
		if err := app.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "用户名或密码错误"})
			return
		}
		if !auth.ComparePassword(user.PasswordHash, req.Password) {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "用户名或密码错误"})
			return
		}
		token, err := app.JWT.Sign(user.Model.ID, user.Username, user.SessionVersion)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "签发令牌失败"})
			return
		}
		c.SetCookie(app.Config.Auth.CookieName, token, int(app.Config.JWTExpireDuration().Seconds()), "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"username": user.Username})
	})
	api.GET("/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"name": app.Config.App.Name, "status": "ok"})
	})
}

// registerProtected 注册需要登录后才能访问的核心业务接口。
func registerProtected(api *gin.RouterGroup, app *runtime.App) {
	protected := api.Group("")
	protected.Use(auth.Middleware(app.Config.Auth.CookieName, app.JWT, app.DB))

	protected.GET("/auth/me", func(c *gin.Context) {
		claims := auth.MustClaims(c)
		c.JSON(http.StatusOK, gin.H{"user_id": claims.UserID, "username": claims.Username})
	})
	protected.POST("/auth/logout", func(c *gin.Context) {
		c.SetCookie(app.Config.Auth.CookieName, "", -1, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"message": "已退出登录"})
	})
	protected.POST("/auth/change-password", func(c *gin.Context) {
		claims := auth.MustClaims(c)
		var req struct {
			CurrentPassword string `json:"current_password" binding:"required"`
			NewPassword     string `json:"new_password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if utf8.RuneCountInString(req.NewPassword) < 8 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "新密码长度不能少于 8 位"})
			return
		}
		if req.CurrentPassword == req.NewPassword {
			c.JSON(http.StatusBadRequest, gin.H{"message": "新密码不能与当前密码相同"})
			return
		}

		var user model.User
		if err := app.DB.First(&user, claims.UserID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "读取当前用户失败"})
			return
		}
		if !auth.ComparePassword(user.PasswordHash, req.CurrentPassword) {
			c.JSON(http.StatusBadRequest, gin.H{"message": "当前密码不正确"})
			return
		}
		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "生成新密码失败"})
			return
		}
		if err := app.DB.Model(&user).Updates(map[string]any{
			"password_hash":   hash,
			"session_version": gorm.Expr("session_version + 1"),
		}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "保存新密码失败"})
			return
		}
		c.SetCookie(app.Config.Auth.CookieName, "", -1, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"message": "密码修改成功，请重新登录"})
	})
	protected.GET("/account-providers", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items":                   mail.ProviderPresets(),
			"microsoft_oauth_enabled": app.Mailer.MicrosoftOAuthEnabled(),
		})
	})
	protected.GET("/accounts/oauth/microsoft/config", func(c *gin.Context) {
		tenantID := strings.TrimSpace(app.Config.MicrosoftOAuth.TenantID)
		if tenantID == "" {
			tenantID = "common"
		}
		redirectURL := resolveMicrosoftOAuthFrontendRedirectURL(c, app)
		c.JSON(http.StatusOK, gin.H{
			"enabled":      app.Mailer.MicrosoftOAuthEnabled(),
			"client_id":    app.Config.MicrosoftOAuth.ClientID,
			"tenant_id":    tenantID,
			"redirect_uri": redirectURL,
			"scope":        mail.MicrosoftOAuthScope(),
			"flow":         microsoftOAuthFlow(redirectURL),
		})
	})

	protected.GET("/accounts", func(c *gin.Context) {
		var accounts []model.MailAccount
		query := app.DB.Order("id desc")
		if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
			like := "%" + keyword + "%"
			query = query.Where("name LIKE ? OR email LIKE ? OR provider_name LIKE ?", like, like, like)
		}
		if err := query.Find(&accounts).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询邮箱失败"})
			return
		}
		c.JSON(http.StatusOK, accounts)
	})

	protected.GET("/preferences/theme", func(c *gin.Context) {
		claims := auth.MustClaims(c)
		preference, err := loadThemePreference(app.DB, claims.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询主题设置失败"})
			return
		}
		c.JSON(http.StatusOK, preference)
	})

	protected.PUT("/preferences/theme", func(c *gin.Context) {
		claims := auth.MustClaims(c)
		var req struct {
			ThemeName      string `json:"theme_name" binding:"required"`
			ThemeMode      string `json:"theme_mode" binding:"required"`
			PrimaryColor   string `json:"primary_color" binding:"required"`
			SecondaryColor string `json:"secondary_color" binding:"required"`
			AccentColor    string `json:"accent_color" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if req.ThemeMode != "light" && req.ThemeMode != "dark" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "主题模式仅支持 light 或 dark"})
			return
		}
		if !isHexColor(req.PrimaryColor) || !isHexColor(req.SecondaryColor) || !isHexColor(req.AccentColor) {
			c.JSON(http.StatusBadRequest, gin.H{"message": "主题颜色格式不合法"})
			return
		}
		preference, err := loadThemePreference(app.DB, claims.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "读取主题设置失败"})
			return
		}
		preference.ThemeName = strings.TrimSpace(req.ThemeName)
		preference.ThemeMode = req.ThemeMode
		preference.PrimaryColor = strings.TrimSpace(req.PrimaryColor)
		preference.SecondaryColor = strings.TrimSpace(req.SecondaryColor)
		preference.AccentColor = strings.TrimSpace(req.AccentColor)
		if err := app.DB.Save(preference).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "保存主题设置失败"})
			return
		}
		c.JSON(http.StatusOK, preference)
	})

	protected.POST("/accounts", func(c *gin.Context) {
		var input mail.AccountInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		account := &model.MailAccount{}
		if err := app.Mailer.SaveAccount(account, input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, account)
	})

	protected.POST("/accounts/import", func(c *gin.Context) {
		var req struct {
			Items []mail.AccountInput `json:"items" binding:"required,min=1"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "导入参数不合法"})
			return
		}
		accounts := make([]model.MailAccount, 0, len(req.Items))
		if err := app.DB.Transaction(func(tx *gorm.DB) error {
			mailer := app.Mailer.WithDB(tx)
			for index, item := range req.Items {
				if strings.EqualFold(strings.TrimSpace(item.AuthType), "oauth") {
					return fmt.Errorf("第 %d 条记录使用了 OAuth，批量导入仅支持非 OAuth 邮箱", index+1)
				}
				account := &model.MailAccount{}
				if err := mailer.SaveAccount(account, item); err != nil {
					return fmt.Errorf("第 %d 条记录导入失败: %s", index+1, err.Error())
				}
				accounts = append(accounts, *account)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": accounts, "message": fmt.Sprintf("成功导入 %d 个邮箱", len(accounts))})
	})

	protected.GET("/accounts/oauth/microsoft/start", func(c *gin.Context) {
		state, err := mail.CreateOAuthState()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "生成 OAuth state 失败"})
			return
		}
		loginHint := strings.TrimSpace(c.Query("login_hint"))
		redirectURL, err := app.Mailer.BuildMicrosoftPKCEOAuthURL(state, resolveMicrosoftOAuthLegacyRedirectURL(c, app), "", loginHint)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.SetCookie("gmbox_oauth_state", state, 600, "/", "", false, true)
		if c.Query("popup") == "1" {
			c.SetCookie("gmbox_oauth_popup", "1", 600, "/", "", false, true)
		} else {
			c.SetCookie("gmbox_oauth_popup", "", -1, "/", "", false, true)
		}
		c.Redirect(http.StatusFound, redirectURL)
	})

	protected.GET("/accounts/oauth/microsoft/callback", func(c *gin.Context) {
		popup := isOAuthPopup(c)
		state, err := c.Cookie("gmbox_oauth_state")
		if err != nil || state == "" || state != c.Query("state") {
			respondOAuthResult(c, popup, false, "微软 OAuth state 校验失败，请重试")
			return
		}
		c.SetCookie("gmbox_oauth_state", "", -1, "/", "", false, true)
		c.SetCookie("gmbox_oauth_popup", "", -1, "/", "", false, true)
		if queryErr := strings.TrimSpace(c.Query("error")); queryErr != "" {
			respondOAuthResult(c, popup, false, queryErr)
			return
		}
		code := strings.TrimSpace(c.Query("code"))
		if code == "" {
			respondOAuthResult(c, popup, false, "微软 OAuth 未返回授权码")
			return
		}
		account, err := app.Mailer.UpsertMicrosoftOAuthAccount(c.Request.Context(), code, resolveMicrosoftOAuthLegacyRedirectURL(c, app))
		if err != nil {
			respondOAuthResult(c, popup, false, err.Error())
			return
		}
		respondOAuthResult(c, popup, true, fmt.Sprintf("微软 OAuth 登录成功，已接入邮箱 %s", account.Email))
	})
	protected.POST("/accounts/oauth/microsoft/exchange", func(c *gin.Context) {
		var req struct {
			Code         string `json:"code" binding:"required"`
			CodeVerifier string `json:"code_verifier" binding:"required"`
			State        string `json:"state" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		account, err := app.Mailer.UpsertMicrosoftOAuthAccountWithPKCE(
			c.Request.Context(),
			strings.TrimSpace(req.Code),
			strings.TrimSpace(req.CodeVerifier),
			resolveMicrosoftOAuthFrontendRedirectURL(c, app),
		)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("微软 OAuth 登录成功，已接入邮箱 %s", account.Email),
			"account": account,
		})
	})

	protected.POST("/accounts/sync", func(c *gin.Context) {
		var req struct {
			IDs []uint `json:"ids" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		var accounts []model.MailAccount
		if err := app.DB.Where("id IN ?", req.IDs).Find(&accounts).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询邮箱失败"})
			return
		}
		if len(accounts) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "未找到可同步的邮箱"})
			return
		}
		if err := app.Syncer.RunAccountsNow(c.Request.Context(), accounts); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "批量同步已完成"})
	})

	protected.PUT("/accounts/:id", func(c *gin.Context) {
		account, ok := loadAccount(c, app.DB)
		if !ok {
			return
		}
		var input mail.AccountInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if err := app.Mailer.SaveAccount(account, input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, account)
	})

	protected.DELETE("/accounts/:id", func(c *gin.Context) {
		account, ok := loadAccount(c, app.DB)
		if !ok {
			return
		}
		if err := deleteAccountCascade(app.DB, account.Model.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "删除邮箱失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
	})

	protected.POST("/accounts/:id/test", func(c *gin.Context) {
		account, ok := loadAccount(c, app.DB)
		if !ok {
			return
		}
		if err := app.Mailer.TestConnection(c.Request.Context(), *account); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "连接测试成功"})
	})

	protected.POST("/accounts/:id/sync", func(c *gin.Context) {
		account, ok := loadAccount(c, app.DB)
		if !ok {
			return
		}
		if err := app.Syncer.RunAccountNow(c.Request.Context(), *account); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "同步已完成"})
	})

	protected.GET("/messages", func(c *gin.Context) {
		var messages []model.Message
		query := app.DB.Model(&model.Message{}).Where("is_deleted = ?", false)
		if accountID := c.Query("account_id"); accountID != "" {
			query = query.Where("account_id = ?", accountID)
		}
		if folder := c.Query("folder"); folder != "" {
			query = query.Where("folder = ?", folder)
		}
		if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
			like := "%" + keyword + "%"
			query = query.Where(
				"subject LIKE ? OR from_name LIKE ? OR from_address LIKE ? OR snippet LIKE ?",
				like,
				like,
				like,
				like,
			)
		}

		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(app.Config.Mail.PageSize)))
		if pageSize < 1 {
			pageSize = app.Config.Mail.PageSize
		}
		if pageSize > 200 {
			pageSize = 200
		}

		var total int64
		if err := query.Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "统计邮件失败"})
			return
		}
		if err := query.Order("sent_at desc, id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&messages).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询邮件失败"})
			return
		}
		items, err := buildMessageResponses(app.DB, messages)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "补充邮件账户信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		})
	})

	protected.GET("/mailboxes", func(c *gin.Context) {
		accountID, _ := strconv.Atoi(c.DefaultQuery("account_id", "0"))
		mailboxes, err := app.Mailer.ListMailboxes(uint(accountID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询文件夹失败"})
			return
		}
		c.JSON(http.StatusOK, mailboxes)
	})

	protected.POST("/contacts/aggregate", func(c *gin.Context) {
		var req struct {
			PrimaryAddress string   `json:"primary_address" binding:"required"`
			Addresses      []string `json:"addresses" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if err := aggregateContacts(app.DB, req.PrimaryAddress, req.Addresses); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "联系人聚合成功"})
	})

	protected.PUT("/contacts/aggregate", func(c *gin.Context) {
		var req struct {
			CurrentAddress string   `json:"current_address" binding:"required"`
			PrimaryAddress string   `json:"primary_address" binding:"required"`
			Addresses      []string `json:"addresses" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if err := updateContactAggregation(app.DB, req.CurrentAddress, req.PrimaryAddress, req.Addresses); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "联系人聚合更新成功"})
	})

	protected.POST("/contacts/separate", func(c *gin.Context) {
		var req struct {
			Addresses []string `json:"addresses" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if err := separateContacts(app.DB, req.Addresses); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "联系人分离成功"})
	})

	protected.POST("/contacts/blacklist", func(c *gin.Context) {
		var req struct {
			Addresses []string `json:"addresses" binding:"required,min=1"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if err := blockContacts(app.DB, req.Addresses); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "联系人已加入黑名单"})
	})

	protected.DELETE("/contacts/blacklist", func(c *gin.Context) {
		var req struct {
			Addresses []string `json:"addresses" binding:"required,min=1"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if err := unblockContacts(app.DB, req.Addresses); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "联系人已移出黑名单"})
	})

	protected.GET("/contacts", func(c *gin.Context) {
		page, pageSize := parsePageParams(c, app.Config.Mail.PageSize)
		contacts, err := loadContactGroups(app.DB, strings.TrimSpace(c.Query("keyword")))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询联系人失败"})
			return
		}
		total := int64(len(contacts))
		start := (page - 1) * pageSize
		if start > len(contacts) {
			start = len(contacts)
		}
		end := start + pageSize
		if end > len(contacts) {
			end = len(contacts)
		}
		contacts = contacts[start:end]
		c.JSON(http.StatusOK, gin.H{"items": contacts, "total": total, "page": page, "page_size": pageSize})
	})

	protected.GET("/contact-messages", func(c *gin.Context) {
		address := normalizeContactAddress(c.Query("address"))
		page, pageSize := parsePageParams(c, app.Config.Mail.PageSize)
		query := app.DB.Model(&model.Message{}).Where("is_deleted = ?", false)

		var candidates []model.Message
		if err := query.Order("sent_at desc, id desc").Find(&candidates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询联系人邮件失败"})
			return
		}
		messages := candidates
		if address != "" {
			addresses, err := expandContactAddresses(app.DB, []string{address})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "查询联系人聚合关系失败"})
				return
			}
			messages = filterContactMessages(candidates, addresses)
		}
		total := int64(len(messages))
		start := (page - 1) * pageSize
		if start > len(messages) {
			start = len(messages)
		}
		end := start + pageSize
		if end > len(messages) {
			end = len(messages)
		}
		messages = messages[start:end]
		items, err := buildMessageResponses(app.DB, messages)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "补充联系人邮件账户信息失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
	})

	protected.GET("/messages/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		message, body, attachments, err := app.Mailer.GetMessageDetail(c.Request.Context(), uint(id))
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"message": "邮件不存在"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		accountEmail, err := loadAccountEmail(app.DB, message.AccountID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询邮件所属邮箱失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": newMessageResponse(*message, accountEmail), "body": body, "attachments": attachments})
	})

	protected.POST("/messages/send", func(c *gin.Context) {
		var input mail.SendInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if err := app.Mailer.Send(c.Request.Context(), input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "发送成功"})
	})

	protected.POST("/messages/:id/read", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		if err := app.Mailer.SetMessageRead(c.Request.Context(), uint(id), true); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已标记为已读"})
	})

	protected.POST("/messages/:id/unread", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		if err := app.Mailer.SetMessageRead(c.Request.Context(), uint(id), false); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已标记为未读"})
	})

	protected.POST("/messages/:id/delete", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		if err := app.Mailer.DeleteMessage(c.Request.Context(), uint(id)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
	})

	protected.POST("/messages/:id/move", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		var req struct {
			Folder string `json:"folder" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "目标文件夹不能为空"})
			return
		}
		if err := app.Mailer.MoveMessage(c.Request.Context(), uint(id), req.Folder); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "移动成功"})
	})

	protected.GET("/attachments/:id/download", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "附件 ID 不合法"})
			return
		}
		attachment, content, err := app.Mailer.DownloadAttachment(uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"message": "附件不存在"})
			return
		}
		contentType := attachment.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", attachment.FileName))
		c.Data(http.StatusOK, contentType, content)
	})

	protected.GET("/sync-states", func(c *gin.Context) {
		var states []model.SyncState
		if err := app.DB.Order("id desc").Find(&states).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询同步状态失败"})
			return
		}
		c.JSON(http.StatusOK, states)
	})

	protected.GET("/sync-logs", func(c *gin.Context) {
		var logs []model.SyncLog
		query := app.DB.Model(&model.SyncLog{}).Order("started_at desc, id desc")
		page, pageSize := parsePageParams(c, 20)
		var total int64
		if err := query.Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "统计同步日志失败"})
			return
		}
		if err := query.Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询同步日志失败"})
			return
		}
		items := make([]syncLogResponse, 0, len(logs))
		for _, item := range logs {
			items = append(items, buildSyncLogResponse(item))
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
	})
}

// syncLogResponse 统一把聚合日志和邮箱明细整理成前端直接可用的结构。
type syncLogResponse struct {
	model.SyncLog
	Details []model.SyncLogDetail `json:"details"`
}

// buildSyncLogResponse 把聚合日志明细解析成前端直接可用的结构。
func buildSyncLogResponse(item model.SyncLog) syncLogResponse {
	response := syncLogResponse{SyncLog: item, Details: []model.SyncLogDetail{}}
	if strings.TrimSpace(item.Details) == "" {
		return response
	}
	if err := json.Unmarshal([]byte(item.Details), &response.Details); err != nil {
		response.Details = []model.SyncLogDetail{}
	}
	if response.AccountCount == 0 && len(response.Details) > 0 {
		response.AccountCount = len(response.Details)
	}
	if response.SuccessCount == 0 && response.AccountCount > 0 {
		for _, item := range response.Details {
			if item.Success {
				response.SuccessCount++
			}
		}
	}
	if response.SuccessRate == 0 && response.AccountCount > 0 {
		response.SuccessRate = float64(response.SuccessCount) / float64(response.AccountCount) * 100
	}
	return response
}

// contactMember 用于向前端展示聚合组里包含的具体联系人，方便查看和分离。
type contactMember struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

// contactItem 表示联系人页的单个聚合结果，主联系人本身也会作为只有一个成员的组返回。
type contactItem struct {
	Address      string           `json:"address"`
	Name         string           `json:"name"`
	LatestSentAt contactTimestamp `json:"latest_sent_at"`
	Total        int64            `json:"total"`
	MemberCount  int              `json:"member_count"`
	Members      []contactMember  `json:"members"`
	IsBlocked    bool             `json:"is_blocked"`
}

// contactSummary 保存按原始发件人聚合后的基础联系人数据，便于后续再做联系人聚合计算。
type contactSummary struct {
	Address      string
	Name         string
	LatestSentAt contactTimestamp
	Total        int64
}

// contactTimestamp 兼容不同数据库驱动对聚合时间列返回 string 或 time.Time 的差异。
type contactTimestamp string

// Scan 统一把联系人最近发信时间转换为字符串，避免聚合列在不同驱动下扫描失败。
func (t *contactTimestamp) Scan(value any) error {
	if value == nil {
		*t = ""
		return nil
	}
	switch typed := value.(type) {
	case time.Time:
		*t = contactTimestamp(typed.UTC().Format(time.RFC3339Nano))
		return nil
	case []byte:
		*t = contactTimestamp(strings.TrimSpace(string(typed)))
		return nil
	case string:
		*t = contactTimestamp(strings.TrimSpace(typed))
		return nil
	default:
		return fmt.Errorf("不支持的联系人时间类型: %T", value)
	}
}

// loadContactGroups 先按原始发件人汇总，再按联系人聚合关系折叠成前端可直接展示的组。
func loadContactGroups(db *gorm.DB, keyword string) ([]contactItem, error) {
	summaries, err := loadContactSummaries(db)
	if err != nil {
		return nil, err
	}
	mappings, err := loadContactAggregationMap(db)
	if err != nil {
		return nil, err
	}
	groups := groupContacts(summaries, mappings)
	blacklist, err := loadContactBlacklistSet(db)
	if err != nil {
		return nil, err
	}
	groups = ensureBlockedContactsVisible(groups, mappings, blacklist)
	applyContactBlockedState(groups, blacklist)
	if strings.TrimSpace(keyword) != "" {
		groups = filterContactGroupsByKeyword(groups, keyword)
	}
	return groups, nil
}

// loadContactBlacklistSet 统一读取联系人黑名单，避免列表展示和同步过滤各自维护不同口径。
func loadContactBlacklistSet(db *gorm.DB) (map[string]struct{}, error) {
	var items []model.ContactBlacklist
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		address := normalizeContactAddress(item.Address)
		if address == "" {
			continue
		}
		result[address] = struct{}{}
	}
	return result, nil
}

// applyContactBlockedState 仅当整个联系人组都已被拉黑时才标记组状态，避免部分成员被误展示成整组已拉黑。
func applyContactBlockedState(groups []contactItem, blacklist map[string]struct{}) {
	for index := range groups {
		if len(groups[index].Members) == 0 {
			_, groups[index].IsBlocked = blacklist[groups[index].Address]
			continue
		}
		groups[index].IsBlocked = true
		for _, member := range groups[index].Members {
			if _, ok := blacklist[member.Address]; ok {
				continue
			}
			groups[index].IsBlocked = false
			break
		}
	}
}

// ensureBlockedContactsVisible 把仅存在于黑名单中的联系人也补进列表，避免无历史邮件后失去解除拉黑入口。
func ensureBlockedContactsVisible(groups []contactItem, mappings map[string]string, blacklist map[string]struct{}) []contactItem {
	if len(blacklist) == 0 {
		return groups
	}
	groupMap := make(map[string]*contactItem, len(groups))
	for index := range groups {
		group := groups[index]
		groupMap[group.Address] = &groups[index]
	}
	for _, address := range uniqueContactAddresses(append(contactMappingAddresses(mappings), mapKeys(blacklist)...)) {
		if _, ok := blacklist[address]; !ok {
			continue
		}
		root := resolveContactPrimary(address, mappings)
		item, ok := groupMap[root]
		if !ok {
			groups = append(groups, contactItem{Address: root, Members: make([]contactMember, 0, 1)})
			item = &groups[len(groups)-1]
			groupMap[root] = item
		}
		appendContactMember(item, contactMember{Address: address})
	}
	for index := range groups {
		if groups[index].MemberCount > 0 {
			continue
		}
		sort.Slice(groups[index].Members, func(i int, j int) bool {
			left := groups[index].Members[i]
			right := groups[index].Members[j]
			if left.Address == groups[index].Address {
				return true
			}
			if right.Address == groups[index].Address {
				return false
			}
			return left.Address < right.Address
		})
		groups[index].MemberCount = len(groups[index].Members)
		groups[index].Name = chooseContactGroupName(groups[index])
	}
	sort.Slice(groups, func(i int, j int) bool {
		if groups[i].LatestSentAt != groups[j].LatestSentAt {
			return isContactTimestampAfter(groups[i].LatestSentAt, groups[j].LatestSentAt)
		}
		return groups[i].Address < groups[j].Address
	})
	return groups
}

// mapKeys 把集合键拉平成切片，避免为补齐黑名单联系人重复写一套遍历逻辑。
func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

// loadContactSummaries 只从真实邮件发件人中提取联系人，避免把聚合逻辑耦合进数据库方言相关 SQL。
func loadContactSummaries(db *gorm.DB) ([]contactSummary, error) {
	var summaries []contactSummary
	err := db.Table("messages").
		Select("from_address AS address, MAX(from_name) AS name, MAX(sent_at) AS latest_sent_at, COUNT(*) AS total").
		Where("is_deleted = ? AND TRIM(from_address) <> ''", false).
		Where("from_address NOT IN (?)", db.Model(&model.MailAccount{}).Select("email")).
		Group("from_address").
		Find(&summaries).Error
	if err != nil {
		return nil, err
	}
	return summaries, nil
}

// loadContactAggregationMap 统一读取聚合映射并做地址归一化，避免大小写不同导致同一邮箱被拆成多个组。
func loadContactAggregationMap(db *gorm.DB) (map[string]string, error) {
	var items []model.ContactAggregation
	if err := db.Find(&items).Error; err != nil {
		return nil, err
	}
	mappings := make(map[string]string, len(items))
	for _, item := range items {
		address := normalizeContactAddress(item.Address)
		primary := normalizeContactAddress(item.PrimaryAddress)
		if address == "" || primary == "" || address == primary {
			continue
		}
		mappings[address] = primary
	}
	return mappings, nil
}

// groupContacts 把原始联系人按主联系人折叠，确保列表统计和成员清单始终以同一套映射关系计算。
func groupContacts(summaries []contactSummary, mappings map[string]string) []contactItem {
	groupMap := make(map[string]*contactItem)
	for _, summary := range summaries {
		address := normalizeContactAddress(summary.Address)
		if address == "" {
			continue
		}
		root := resolveContactPrimary(address, mappings)
		item, ok := groupMap[root]
		if !ok {
			item = &contactItem{Address: root, Members: make([]contactMember, 0, 1)}
			groupMap[root] = item
		}
		item.Total += summary.Total
		if isContactTimestampAfter(summary.LatestSentAt, item.LatestSentAt) {
			item.LatestSentAt = summary.LatestSentAt
		}
		appendContactMember(item, contactMember{Address: address, Name: strings.TrimSpace(summary.Name)})
	}
	for _, address := range uniqueContactAddresses(contactMappingAddresses(mappings)) {
		root := resolveContactPrimary(address, mappings)
		item, ok := groupMap[root]
		if !ok {
			item = &contactItem{Address: root, Members: make([]contactMember, 0, 1)}
			groupMap[root] = item
		}
		appendContactMember(item, contactMember{Address: address})
	}

	groups := make([]contactItem, 0, len(groupMap))
	for _, item := range groupMap {
		sort.Slice(item.Members, func(i int, j int) bool {
			left := item.Members[i]
			right := item.Members[j]
			if left.Address == item.Address {
				return true
			}
			if right.Address == item.Address {
				return false
			}
			return left.Address < right.Address
		})
		item.MemberCount = len(item.Members)
		item.Name = chooseContactGroupName(*item)
		groups = append(groups, *item)
	}

	sort.Slice(groups, func(i int, j int) bool {
		if groups[i].LatestSentAt != groups[j].LatestSentAt {
			return isContactTimestampAfter(groups[i].LatestSentAt, groups[j].LatestSentAt)
		}
		return groups[i].Address < groups[j].Address
	})
	return groups
}

// appendContactMember 合并同一成员的显示信息，确保手动加入但尚无历史邮件的邮箱也能保留在聚合组中。
func appendContactMember(item *contactItem, member contactMember) {
	for index, existing := range item.Members {
		if existing.Address != member.Address {
			continue
		}
		if strings.TrimSpace(item.Members[index].Name) == "" && strings.TrimSpace(member.Name) != "" {
			item.Members[index].Name = strings.TrimSpace(member.Name)
		}
		return
	}
	item.Members = append(item.Members, contactMember{Address: member.Address, Name: strings.TrimSpace(member.Name)})
}

// contactMappingAddresses 把映射里的主成员地址都提取出来，避免手工追加的邮箱因为暂时没有邮件而从聚合组里消失。
func contactMappingAddresses(mappings map[string]string) []string {
	addresses := make([]string, 0, len(mappings)*2)
	for address, primary := range mappings {
		addresses = append(addresses, address, primary)
	}
	return addresses
}

// chooseContactGroupName 优先复用主联系人的名字，避免聚合后列表标题在刷新时来回跳变。
func chooseContactGroupName(item contactItem) string {
	for _, member := range item.Members {
		if member.Address == item.Address && strings.TrimSpace(member.Name) != "" {
			return strings.TrimSpace(member.Name)
		}
	}
	for _, member := range item.Members {
		if strings.TrimSpace(member.Name) != "" {
			return strings.TrimSpace(member.Name)
		}
	}
	return item.Address
}

// filterContactGroupsByKeyword 让搜索同时命中主联系人和聚合成员，避免成员被聚合后无法被检索到。
func filterContactGroupsByKeyword(groups []contactItem, keyword string) []contactItem {
	needle := strings.ToLower(strings.TrimSpace(keyword))
	if needle == "" {
		return groups
	}
	filtered := make([]contactItem, 0, len(groups))
	for _, item := range groups {
		if strings.Contains(strings.ToLower(item.Address), needle) || strings.Contains(strings.ToLower(item.Name), needle) {
			filtered = append(filtered, item)
			continue
		}
		matched := false
		for _, member := range item.Members {
			if strings.Contains(strings.ToLower(member.Address), needle) || strings.Contains(strings.ToLower(member.Name), needle) {
				matched = true
				break
			}
		}
		if matched {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// expandContactAddresses 把传入联系人扩展成所在聚合组的全部成员，保证联系人邮件列表按聚合口径加载。
func expandContactAddresses(db *gorm.DB, addresses []string) ([]string, error) {
	mappings, err := loadContactAggregationMap(db)
	if err != nil {
		return nil, err
	}
	knownAddresses, err := loadKnownContactAddresses(db, mappings)
	if err != nil {
		return nil, err
	}
	return expandContactGroupMembers(addresses, mappings, knownAddresses), nil
}

// loadKnownContactAddresses 汇总真实联系人和聚合映射里的地址，避免分组展开时漏掉仅存在于映射中的成员。
func loadKnownContactAddresses(db *gorm.DB, mappings map[string]string) ([]string, error) {
	summaries, err := loadContactSummaries(db)
	if err != nil {
		return nil, err
	}
	known := make([]string, 0, len(summaries)+len(mappings)*2)
	for _, item := range summaries {
		known = append(known, normalizeContactAddress(item.Address))
	}
	for address, primary := range mappings {
		known = append(known, address, primary)
	}
	return uniqueContactAddresses(known), nil
}

// expandContactGroupMembers 把任意选中的联系人展开成完整分组，便于组合聚合和整组分离时保持结果可预期。
func expandContactGroupMembers(addresses []string, mappings map[string]string, knownAddresses []string) []string {
	groups := buildContactGroupMembers(knownAddresses, mappings)
	expanded := make([]string, 0, len(addresses))
	seen := make(map[string]struct{})
	for _, raw := range addresses {
		address := normalizeContactAddress(raw)
		if address == "" {
			continue
		}
		root := resolveContactPrimary(address, mappings)
		members := groups[root]
		if len(members) == 0 {
			members = []string{address}
		}
		for _, member := range members {
			if _, ok := seen[member]; ok {
				continue
			}
			seen[member] = struct{}{}
			expanded = append(expanded, member)
		}
	}
	return expanded
}

// buildContactGroupMembers 预先构建主联系人到全部成员的索引，避免多次展开时重复遍历映射。
func buildContactGroupMembers(addresses []string, mappings map[string]string) map[string][]string {
	groups := make(map[string][]string)
	for _, raw := range uniqueContactAddresses(addresses) {
		address := normalizeContactAddress(raw)
		if address == "" {
			continue
		}
		root := resolveContactPrimary(address, mappings)
		groups[root] = append(groups[root], address)
	}
	for root := range groups {
		sort.Strings(groups[root])
	}
	return groups
}

// aggregateContacts 把多个联系人或联系人组并到同一个主联系人下，保证后续列表统计和邮件查询统一收口。
func aggregateContacts(db *gorm.DB, primaryAddress string, addresses []string) error {
	primary, err := canonicalContactAddress(primaryAddress)
	if err != nil {
		return err
	}
	mappings, err := loadContactAggregationMap(db)
	if err != nil {
		return fmt.Errorf("查询联系人聚合关系失败: %w", err)
	}
	knownAddresses, err := loadKnownContactAddresses(db, mappings)
	if err != nil {
		return fmt.Errorf("查询联系人失败: %w", err)
	}
	normalizedAddresses := make([]string, 0, len(addresses)+1)
	for _, item := range append(addresses, primary) {
		address, addressErr := canonicalContactAddress(item)
		if addressErr != nil {
			return addressErr
		}
		normalizedAddresses = append(normalizedAddresses, address)
	}
	expanded := expandContactGroupMembers(normalizedAddresses, mappings, knownAddresses)
	members := make([]string, 0, len(expanded))
	for _, item := range expanded {
		if item == primary {
			continue
		}
		members = append(members, item)
	}
	if len(members) == 0 {
		return errors.New("请至少选择两个不同联系人进行聚合")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("address = ?", primary).Delete(&model.ContactAggregation{}).Error; err != nil {
			return err
		}
		for _, member := range members {
			record := model.ContactAggregation{Address: member, PrimaryAddress: primary}
			if err := tx.Where("address = ?", member).Assign(record).FirstOrCreate(&record).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// updateContactAggregation 在一个事务里替换整个聚合组，避免前端编辑时先拆后建造成中途失败后原关系丢失。
func updateContactAggregation(db *gorm.DB, currentAddress string, primaryAddress string, addresses []string) error {
	current, err := canonicalContactAddress(currentAddress)
	if err != nil {
		return err
	}
	primary, err := canonicalContactAddress(primaryAddress)
	if err != nil {
		return err
	}
	mappings, err := loadContactAggregationMap(db)
	if err != nil {
		return fmt.Errorf("查询联系人聚合关系失败: %w", err)
	}
	knownAddresses, err := loadKnownContactAddresses(db, mappings)
	if err != nil {
		return fmt.Errorf("查询联系人失败: %w", err)
	}
	currentMembers := expandContactGroupMembers([]string{current}, mappings, knownAddresses)
	normalizedAddresses := make([]string, 0, len(addresses)+1)
	for _, item := range addresses {
		address, addressErr := canonicalContactAddress(item)
		if addressErr != nil {
			return addressErr
		}
		normalizedAddresses = append(normalizedAddresses, address)
	}
	normalizedAddresses = append(normalizedAddresses, primary)
	nextMembers := uniqueContactAddresses(normalizedAddresses)
	if len(nextMembers) == 0 {
		return errors.New("请选择联系人")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		currentRoots := uniqueContactAddresses(append(currentMembers, current))
		for _, item := range currentRoots {
			root := resolveContactPrimary(item, mappings)
			if root == "" {
				continue
			}
			if err := tx.Where("primary_address = ?", root).Delete(&model.ContactAggregation{}).Error; err != nil {
				return err
			}
		}
		if len(nextMembers) == 1 {
			return tx.Where("address = ?", primary).Delete(&model.ContactAggregation{}).Error
		}
		if err := tx.Where("address = ?", primary).Delete(&model.ContactAggregation{}).Error; err != nil {
			return err
		}
		for _, member := range nextMembers {
			if member == primary {
				continue
			}
			record := model.ContactAggregation{Address: member, PrimaryAddress: primary}
			if err := tx.Where("address = ?", member).Assign(record).FirstOrCreate(&record).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// separateContacts 允许按成员分离，也支持传入主联系人时直接解散整个聚合组。
func separateContacts(db *gorm.DB, addresses []string) error {
	targets := uniqueContactAddresses(addresses)
	if len(targets) == 0 {
		return errors.New("请选择要分离的联系人")
	}
	mappings, err := loadContactAggregationMap(db)
	if err != nil {
		return fmt.Errorf("查询联系人聚合关系失败: %w", err)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, target := range targets {
			root := resolveContactPrimary(target, mappings)
			if root == target {
				if err := tx.Where("primary_address = ?", root).Delete(&model.ContactAggregation{}).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Where("address = ?", target).Delete(&model.ContactAggregation{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// blockContacts 把联系人或整组联系人加入黑名单，后续同步会直接跳过这些发件人。
func blockContacts(db *gorm.DB, addresses []string) error {
	targets, err := expandBlacklistTargets(db, addresses)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("请选择要加入黑名单的联系人")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, address := range targets {
			record := model.ContactBlacklist{Address: address}
			if err := tx.Where("address = ?", address).FirstOrCreate(&record).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// unblockContacts 支持按当前联系人或整组联系人恢复收信，避免前端必须逐个成员取消黑名单。
func unblockContacts(db *gorm.DB, addresses []string) error {
	targets, err := expandBlacklistTargets(db, addresses)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return errors.New("请选择要移出黑名单的联系人")
	}
	return db.Where("address IN ?", targets).Delete(&model.ContactBlacklist{}).Error
}

// expandBlacklistTargets 先做邮箱校验，再按联系人聚合关系展开整组成员，保证黑名单规则与联系人展示口径一致。
func expandBlacklistTargets(db *gorm.DB, addresses []string) ([]string, error) {
	validated := make([]string, 0, len(addresses))
	for _, item := range addresses {
		address, err := canonicalContactAddress(item)
		if err != nil {
			return nil, err
		}
		validated = append(validated, address)
	}
	if len(validated) == 0 {
		return nil, nil
	}
	targets, err := expandContactAddresses(db, validated)
	if err != nil {
		return nil, fmt.Errorf("查询联系人聚合关系失败: %w", err)
	}
	return uniqueContactAddresses(targets), nil
}

// resolveContactPrimary 按映射链找到最终主联系人，并在异常循环时降级返回当前地址避免死循环。
func resolveContactPrimary(address string, mappings map[string]string) string {
	current := normalizeContactAddress(address)
	seen := map[string]struct{}{}
	for current != "" {
		next, ok := mappings[current]
		if !ok || next == "" {
			return current
		}
		if _, duplicated := seen[current]; duplicated {
			return current
		}
		seen[current] = struct{}{}
		current = normalizeContactAddress(next)
	}
	return ""
}

// normalizeContactAddress 统一清理邮箱地址格式，避免大小写和首尾空格造成重复联系人。
func normalizeContactAddress(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// canonicalContactAddress 校验并规范邮箱地址，避免把无效字符串写入聚合映射后污染联系人列表。
func canonicalContactAddress(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", errors.New("邮箱地址不能为空")
	}
	parsed, err := stdmail.ParseAddress(trimmed)
	if err != nil {
		return "", errors.New("邮箱地址格式不合法")
	}
	return normalizeContactAddress(parsed.Address), nil
}

// uniqueContactAddresses 保持输入顺序去重，避免前端多选和整组展开后出现重复成员。
func uniqueContactAddresses(addresses []string) []string {
	result := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, raw := range addresses {
		address := normalizeContactAddress(raw)
		if address == "" {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		result = append(result, address)
	}
	return result
}

// isContactTimestampAfter 统一比较联系人时间，避免不同数据库驱动字符串格式差异导致排序不稳定。
func isContactTimestampAfter(left contactTimestamp, right contactTimestamp) bool {
	leftText := strings.TrimSpace(string(left))
	rightText := strings.TrimSpace(string(right))
	if rightText == "" {
		return leftText != ""
	}
	if leftText == "" {
		return false
	}
	leftTime, leftErr := time.Parse(time.RFC3339Nano, leftText)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, rightText)
	if leftErr == nil && rightErr == nil {
		return leftTime.After(rightTime)
	}
	return leftText > rightText
}

// registerFrontend 让非 API 请求统一交给前端路由处理。
func registerFrontend(router *gin.Engine, assets fs.FS) {
	fileServer := http.FileServer(http.FS(assets))
	router.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet || len(c.Request.URL.Path) >= 5 && c.Request.URL.Path[:5] == "/api/" {
			c.JSON(http.StatusNotFound, gin.H{"message": "未找到资源"})
			return
		}
		if _, err := fs.Stat(assets, c.Request.URL.Path[1:]); err == nil && c.Request.URL.Path != "/" {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, fmt.Sprintf("读取前端入口失败: %v", err))
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
	router.StaticFS("/assets", http.FS(assets))
}

// loadAccount 统一解析邮箱 ID 并查询数据库，减少重复分支。
func loadAccount(c *gin.Context, db *gorm.DB) (*model.MailAccount, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "邮箱 ID 不合法"})
		return nil, false
	}
	var account model.MailAccount
	if err := db.First(&account, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "邮箱不存在"})
			return nil, false
		}
		c.JSON(http.StatusNotFound, gin.H{"message": "邮箱不存在"})
		return nil, false
	}
	return &account, true
}

// redirectAccounts 统一把 OAuth 结果带回邮箱管理页，避免前端再额外处理回调页。
func redirectAccounts(c *gin.Context, key string, value string) {
	query := fmt.Sprintf("/accounts?%s=%s", key, url.QueryEscape(value))
	c.Redirect(http.StatusFound, query)
}

// resolveMicrosoftOAuthFrontendRedirectURL 优先使用显式配置，未配置时按当前访问入口动态推导前端回调地址。
func resolveMicrosoftOAuthFrontendRedirectURL(c *gin.Context, app *runtime.App) string {
	configured := strings.TrimSpace(app.Config.MicrosoftOAuth.RedirectURL)
	if configured != "" {
		return configured
	}
	return requestBaseURL(c) + "/oauth/microsoft/callback"
}

// resolveMicrosoftOAuthLegacyRedirectURL 优先使用显式配置的旧 API 回调地址，未配置时再按当前访问入口推导。
func resolveMicrosoftOAuthLegacyRedirectURL(c *gin.Context, app *runtime.App) string {
	configured := strings.TrimSpace(app.Config.MicrosoftOAuth.RedirectURL)
	if configured != "" {
		return configured
	}
	return requestBaseURL(c) + "/api/accounts/oauth/microsoft/callback"
}

// microsoftOAuthFlow 根据回调地址判断当前应使用 PKCE 还是旧服务端兼容流。
func microsoftOAuthFlow(redirectURL string) string {
	if strings.HasSuffix(strings.TrimSpace(redirectURL), "/api/accounts/oauth/microsoft/callback") {
		return "legacy"
	}
	return "pkce"
}

// isOAuthPopup 判断旧服务端流是否由弹窗发起，以便回调时选择 postMessage 还是页面重定向。
func isOAuthPopup(c *gin.Context) bool {
	value, _ := c.Cookie("gmbox_oauth_popup")
	return strings.TrimSpace(value) == "1"
}

// respondOAuthResult 统一兼容弹窗回调与页面回跳两种结果返回方式。
func respondOAuthResult(c *gin.Context, popup bool, success bool, message string) {
	if !popup {
		if success {
			redirectAccounts(c, "oauth_success", message)
			return
		}
		redirectAccounts(c, "oauth_error", message)
		return
	}
	payload, err := json.Marshal(gin.H{
		"type":    "microsoft-oauth",
		"success": success,
		"message": message,
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "OAuth 结果序列化失败")
		return
	}
	html := fmt.Sprintf(`<!doctype html><html><body><script>
window.opener && window.opener.postMessage(%s, window.location.origin);
window.close();
</script></body></html>`, payload)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// requestBaseURL 优先尊重反向代理透传的协议和主机，避免 OAuth 回调地址落成内网地址。
func requestBaseURL(c *gin.Context) string {
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

var hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// messageResponse 为消息接口补充所属接入邮箱，避免前端把原始收件人误展示成当前账户邮箱。
type messageResponse struct {
	model.Message
	AccountEmail string `json:"account_email"`
}

// parsePageParams 统一解析分页参数，避免各接口重复维护边界判断。
func parsePageParams(c *gin.Context, defaultPageSize int) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultPageSize)))
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

// buildMessageResponses 为一批邮件补齐账户邮箱，保证列表展示口径与实际接入账户一致。
func buildMessageResponses(db *gorm.DB, messages []model.Message) ([]messageResponse, error) {
	if len(messages) == 0 {
		return []messageResponse{}, nil
	}
	accountEmails, err := loadAccountEmails(db, messages)
	if err != nil {
		return nil, err
	}
	items := make([]messageResponse, 0, len(messages))
	for _, item := range messages {
		items = append(items, newMessageResponse(item, accountEmails[item.AccountID]))
	}
	return items, nil
}

// loadAccountEmails 批量读取账户邮箱，避免消息列表逐条查询带来额外数据库压力。
func loadAccountEmails(db *gorm.DB, messages []model.Message) (map[uint]string, error) {
	accountIDs := make([]uint, 0, len(messages))
	seen := make(map[uint]struct{}, len(messages))
	for _, item := range messages {
		if _, ok := seen[item.AccountID]; ok {
			continue
		}
		seen[item.AccountID] = struct{}{}
		accountIDs = append(accountIDs, item.AccountID)
	}
	if len(accountIDs) == 0 {
		return map[uint]string{}, nil
	}
	var accounts []struct {
		ID    uint
		Email string
	}
	if err := db.Model(&model.MailAccount{}).Select("id", "email").Where("id IN ?", accountIDs).Find(&accounts).Error; err != nil {
		return nil, err
	}
	accountEmails := make(map[uint]string, len(accounts))
	for _, item := range accounts {
		accountEmails[item.ID] = item.Email
	}
	return accountEmails, nil
}

// loadAccountEmail 为详情页补齐所属接入邮箱。
func loadAccountEmail(db *gorm.DB, accountID uint) (string, error) {
	if accountID == 0 {
		return "", nil
	}
	var account model.MailAccount
	if err := db.Select("email").First(&account, accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return account.Email, nil
}

// newMessageResponse 统一构造前端消息响应，避免多处手工拼装字段。
func newMessageResponse(message model.Message, accountEmail string) messageResponse {
	return messageResponse{Message: message, AccountEmail: accountEmail}
}

// deleteAccountCascade 在删除邮箱时一并清理邮件、正文和附件，避免留下孤儿数据。
func deleteAccountCascade(db *gorm.DB, accountID uint) error {
	if accountID == 0 {
		return nil
	}
	attachmentPaths := make([]string, 0)
	if err := db.Transaction(func(tx *gorm.DB) error {
		var attachments []model.Attachment
		if err := tx.
			Table("attachments").
			Joins("JOIN messages ON messages.id = attachments.message_id").
			Where("messages.account_id = ?", accountID).
			Find(&attachments).Error; err != nil {
			return err
		}
		for _, item := range attachments {
			if strings.TrimSpace(item.StoragePath) != "" {
				attachmentPaths = append(attachmentPaths, item.StoragePath)
			}
		}

		messageSubQuery := tx.Model(&model.Message{}).Select("id").Where("account_id = ?", accountID)
		if err := tx.Where("message_id IN (?)", messageSubQuery).Unscoped().Delete(&model.Attachment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("message_id IN (?)", messageSubQuery).Unscoped().Delete(&model.MessageBody{}).Error; err != nil {
			return err
		}
		if err := tx.Where("account_id = ?", accountID).Unscoped().Delete(&model.Message{}).Error; err != nil {
			return err
		}
		if err := tx.Where("account_id = ?", accountID).Unscoped().Delete(&model.Mailbox{}).Error; err != nil {
			return err
		}
		if err := tx.Where("account_id = ?", accountID).Unscoped().Delete(&model.SyncState{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&model.MailAccount{}, accountID).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	for _, path := range attachmentPaths {
		_ = os.Remove(path)
	}
	return nil
}

// loadThemePreference 统一兜底主题设置，避免前端首次进入时拿到空配置。
func loadThemePreference(db *gorm.DB, userID uint) (*model.UserPreference, error) {
	preference := &model.UserPreference{UserID: userID}
	err := db.Where("user_id = ?", userID).First(preference).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &model.UserPreference{
			UserID:         userID,
			ThemeName:      "classic_blue",
			ThemeMode:      "light",
			PrimaryColor:   "#2563eb",
			SecondaryColor: "#7c3aed",
			AccentColor:    "#06b6d4",
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return preference, nil
}

// isHexColor 只允许标准十六进制颜色，避免把非法值写进主题配置后污染整个界面。
func isHexColor(value string) bool {
	return hexColorPattern.MatchString(strings.TrimSpace(value))
}

// filterContactMessages 对聚合组里的全部联系人做精确匹配，避免联系人聚合后列表仍只显示单个成员邮件。
func filterContactMessages(messages []model.Message, addresses []string) []model.Message {
	addressSet := make(map[string]struct{}, len(addresses))
	for _, address := range uniqueContactAddresses(addresses) {
		addressSet[address] = struct{}{}
	}
	if len(addressSet) == 0 {
		return []model.Message{}
	}
	filtered := make([]model.Message, 0, len(messages))
	for _, item := range messages {
		if _, ok := addressSet[normalizeContactAddress(item.FromAddress)]; ok || addressInMessageList(item.ToAddresses, addressSet) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// addressInMessageList 尝试按 RFC822 地址列表解析收件人字段，确保聚合联系人筛选仍按完整邮箱精确匹配。
func addressInMessageList(raw string, targets map[string]struct{}) bool {
	if len(targets) == 0 {
		return false
	}
	list, err := stdmail.ParseAddressList(raw)
	if err != nil {
		return false
	}
	for _, item := range list {
		if _, ok := targets[normalizeContactAddress(item.Address)]; ok {
			return true
		}
	}
	return false
}
