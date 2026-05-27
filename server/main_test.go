package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"mx-mail-api/internal/api"
)

/**
 * TestHealthRoute 校验服务暴露了不依赖外部资源的健康检查端点。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：路由未注册、返回非 200 状态码，或响应结构不符合探针预期时测试失败。
 */
func TestHealthRoute(t *testing.T) {
	router := setupRouter(api.New(nil, nil, nil, nil))

	// 使用 httptest 避免真实监听端口，确保测试不会受本机端口占用影响。
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.Code)
	}

	expectedBody := `{"status":"ok"}`
	if resp.Body.String() != expectedBody {
		t.Fatalf("expected body %s, got %s", expectedBody, resp.Body.String())
	}
}

/**
 * TestAPIDocumentRoute 校验 Gin 直接输出根目录 API.md。
 *
 * 参数：Go 测试框架注入 t。
 * 返回值：无。
 * 失败条件：文档路由未注册、需要登录或未返回 Markdown 内容时测试失败。
 */
func TestAPIDocumentRoute(t *testing.T) {
	tmpDir := t.TempDir()
	serverDir := tmpDir + "/server"
	if err := os.Mkdir(serverDir, 0o755); err != nil {
		t.Fatalf("failed to create server dir: %v", err)
	}
	if err := os.WriteFile(tmpDir+"/API.md", []byte("# test api doc\n"), 0o644); err != nil {
		t.Fatalf("failed to create test api doc: %v", err)
	}
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(serverDir); err != nil {
		t.Fatalf("failed to chdir server dir: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("failed to restore working directory: %v", err)
		}
	}()

	router := setupRouter(api.New(nil, nil, nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/docs/api.md", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body %s", http.StatusOK, resp.Code, resp.Body.String())
	}
	if resp.Body.String() != "# test api doc\n" {
		t.Fatalf("expected api doc markdown, got %q", resp.Body.String())
	}
}
