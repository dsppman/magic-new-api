package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type apiAuthResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func performAPIRouterRequestAsRole(t *testing.T, method string, target string, role int) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("api-router-auth-test"))))
	router.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", "tester")
		session.Set("role", role)
		session.Set("id", 1)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	SetApiRouter(router)

	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	require.Equal(t, http.StatusNoContent, loginRecorder.Code)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("New-Api-User", "1")
	for _, cookie := range loginRecorder.Result().Cookies() {
		request.AddCookie(cookie)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func requireInsufficientPrivilege(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	require.Equal(t, http.StatusOK, recorder.Code)

	var body apiAuthResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.NotEmpty(t, body.Message)
}

func TestAdminCannotOperateChannels(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "add channel", method: http.MethodPost, path: "/api/channel/"},
		{name: "update channel", method: http.MethodPut, path: "/api/channel/"},
		{name: "delete channel", method: http.MethodDelete, path: "/api/channel/1"},
		{name: "test all channels", method: http.MethodGet, path: "/api/channel/test"},
		{name: "test channel", method: http.MethodGet, path: "/api/channel/test/1"},
		{name: "update all balances", method: http.MethodGet, path: "/api/channel/update_balance"},
		{name: "update channel balance", method: http.MethodGet, path: "/api/channel/update_balance/1"},
		{name: "delete disabled channels", method: http.MethodDelete, path: "/api/channel/disabled"},
		{name: "disable tag channels", method: http.MethodPost, path: "/api/channel/tag/disabled"},
		{name: "enable tag channels", method: http.MethodPost, path: "/api/channel/tag/enabled"},
		{name: "edit tag channels", method: http.MethodPut, path: "/api/channel/tag"},
		{name: "fix abilities", method: http.MethodPost, path: "/api/channel/fix"},
		{name: "fetch upstream models", method: http.MethodGet, path: "/api/channel/fetch_models/1"},
		{name: "refresh codex credentials", method: http.MethodPost, path: "/api/channel/1/codex/refresh"},
		{name: "codex usage", method: http.MethodGet, path: "/api/channel/1/codex/usage"},
		{name: "ollama version", method: http.MethodGet, path: "/api/channel/ollama/version/1"},
		{name: "tag models", method: http.MethodGet, path: "/api/channel/tag/models"},
		{name: "batch tag", method: http.MethodPost, path: "/api/channel/batch/tag"},
		{name: "delete batch", method: http.MethodPost, path: "/api/channel/batch"},
		{name: "copy channel", method: http.MethodPost, path: "/api/channel/copy/1"},
		{name: "manage multi keys", method: http.MethodPost, path: "/api/channel/multi_key/manage"},
		{name: "apply upstream updates", method: http.MethodPost, path: "/api/channel/upstream_updates/apply"},
		{name: "detect upstream updates", method: http.MethodPost, path: "/api/channel/upstream_updates/detect"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performAPIRouterRequestAsRole(t, tt.method, tt.path, common.RoleAdminUser)

			requireInsufficientPrivilege(t, recorder)
		})
	}
}

func TestAdminCanReadChannelGroupList(t *testing.T) {
	recorder := performAPIRouterRequestAsRole(t, http.MethodGet, "/api/group/", common.RoleAdminUser)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool     `json:"success"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.NotNil(t, body.Data)
}

func TestAdminCannotReadNonChannelManagementRoutes(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "prefill groups", path: "/api/prefill_group?type=model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performAPIRouterRequestAsRole(t, http.MethodGet, tt.path, common.RoleAdminUser)

			requireInsufficientPrivilege(t, recorder)
		})
	}
}
