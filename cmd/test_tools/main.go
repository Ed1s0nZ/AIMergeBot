package main

import (
	"encoding/json"
	"fmt"
	"log"
	"pr_agent/internal"
	"strings"

	"github.com/xanzy/go-gitlab"
)

func main() {
	// 配置GitLab客户端
	gitlabURL := "https://gitlab.com"
	token := "your-gitlab-token" // 请替换为实际的token
	projectID := 12345678        // 请替换为实际的项目ID

	git, err := gitlab.NewClient(token, gitlab.WithBaseURL(gitlabURL+"/api/v4"))
	if err != nil {
		log.Fatalf("GitLab客户端初始化失败: %v", err)
	}

	fmt.Println("=== 测试新的MCP工具 ===")

	// 测试1: 验证提示词
	fmt.Println("\n1. 验证提示词是否包含新工具")
	testPromptTools()

	// 测试2: 获取项目结构
	fmt.Println("\n2. 测试 gitlab_project_structure")
	testProjectStructure(git, projectID)

	// 测试3: 全局搜索
	fmt.Println("\n3. 测试 gitlab_global_search")
	testGlobalSearch(git, projectID)

	// 测试4: 显示可用的MCP工具
	fmt.Println("\n4. 显示可用的MCP工具")
	showAvailableTools()
}

func testPromptTools() {
	// 创建ReAct审计器实例来获取提示词
	tools := internal.GetAvailableSimplifiedGitLabTools()

	// 获取简化模式提示词
	simplifiedPrompt := fmt.Sprint(tools)

	// 检查是否包含新工具
	newTools := []string{
		"gitlab_project_structure",
		"gitlab_global_search",
	}

	fmt.Println("检查简化模式提示词中的新工具:")
	for _, tool := range newTools {
		if strings.Contains(simplifiedPrompt, tool) {
			fmt.Printf("  ✓ %s - 已包含\n", tool)
		} else {
			fmt.Printf("  ✗ %s - 未包含\n", tool)
		}
	}

	// 检查工具使用策略
	if strings.Contains(simplifiedPrompt, "推荐的分析流程") {
		fmt.Println("  ✓ 工具使用策略 - 已包含")
	} else {
		fmt.Println("  ✗ 工具使用策略 - 未包含")
	}

	// 检查搜索策略示例
	if strings.Contains(simplifiedPrompt, "搜索策略示例") {
		fmt.Println("  ✓ 搜索策略示例 - 已包含")
	} else {
		fmt.Println("  ✗ 搜索策略示例 - 未包含")
	}
}

func testProjectStructure(git *gitlab.Client, projectID int) {
	args := map[string]interface{}{
		"include_content":  true,
		"file_type_filter": "go",
	}

	result := internal.SimplifiedGitLabMCPExecutor(internal.GitLabMCPCall{
		ToolName:  "gitlab_project_structure",
		Arguments: args,
	}, git, projectID)

	if result.Error != "" {
		fmt.Printf("错误: %s\n", result.Error)
		return
	}

	// 解析结果
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content), &data); err != nil {
		fmt.Printf("解析结果失败: %v\n", err)
		return
	}

	fmt.Printf("项目名称: %s\n", data["project_name"])
	fmt.Printf("项目路径: %s\n", data["project_path"])

	if stats, ok := data["statistics"].(map[string]interface{}); ok {
		fmt.Printf("总文件数: %v\n", stats["total_files"])
		fmt.Printf("总目录数: %v\n", stats["total_directories"])
		fmt.Printf("总项目数: %v\n", stats["total_items"])
	}

	// 显示前5个文件
	if structure, ok := data["structure"].([]interface{}); ok {
		fmt.Printf("项目结构预览 (前5个):\n")
		count := 0
		for _, item := range structure {
			if count >= 5 {
				break
			}
			if itemMap, ok := item.(map[string]interface{}); ok {
				itemType := itemMap["type"]
				itemPath := itemMap["path"]
				fmt.Printf("  %s: %s\n", itemType, itemPath)
				count++
			}
		}
	}
}

func testGlobalSearch(git *gitlab.Client, projectID int) {
	args := map[string]interface{}{
		"search_patterns":  []interface{}{"func", "import"},
		"file_patterns":    []interface{}{"*.go"},
		"exclude_patterns": []interface{}{"*_test.go", "vendor/*"},
		"case_sensitive":   false,
		"include_context":  true,
		"context_lines":    2,
	}

	result := internal.SimplifiedGitLabMCPExecutor(internal.GitLabMCPCall{
		ToolName:  "gitlab_global_search",
		Arguments: args,
	}, git, projectID)

	if result.Error != "" {
		fmt.Printf("错误: %s\n", result.Error)
		return
	}

	// 解析结果
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content), &data); err != nil {
		fmt.Printf("解析结果失败: %v\n", err)
		return
	}

	fmt.Printf("搜索模式: %v\n", data["search_patterns"])
	fmt.Printf("总匹配数: %v\n", data["total_matches"])
	fmt.Printf("搜索文件数: %v\n", data["total_files_searched"])

	// 显示前3个搜索结果
	if results, ok := data["results"].([]interface{}); ok {
		fmt.Printf("搜索结果预览 (前3个):\n")
		count := 0
		for _, result := range results {
			if count >= 3 {
				break
			}
			if resultMap, ok := result.(map[string]interface{}); ok {
				filePath := resultMap["file_path"]
				lineNumber := resultMap["line_number"]
				pattern := resultMap["pattern"]
				fmt.Printf("  %s:%v [%s]\n", filePath, lineNumber, pattern)
				count++
			}
		}
	}
}

func showAvailableTools() {
	fmt.Println("简化版MCP工具:")
	tools := internal.GetAvailableSimplifiedGitLabTools()
	for i, tool := range tools {
		fmt.Printf("  %d. %s: %s\n", i+1, tool.Name, tool.Description)
	}

	fmt.Println("\n完整版MCP工具:")
	fullTools := internal.GetAvailableGitLabTools()
	for i, tool := range fullTools {
		fmt.Printf("  %d. %s: %s\n", i+1, tool.Name, tool.Description)
	}
}
