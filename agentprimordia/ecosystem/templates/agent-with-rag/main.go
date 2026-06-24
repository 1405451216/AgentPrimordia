// {{.ProjectName}} — Agent with RAG template
// 展示如何在 Agent 上启用知识库检索 (RAG),让 Agent 在回答前
// 自动查询相关文档并注入上下文。
//
// 前置条件：
//
//	export OPENAI_API_KEY=sk-xxx
//	准备 ./knowledge/*.txt 知识库文件
//
// 跑法：
//
//	cd {{.ProjectName}}
//	go mod tidy
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	ap "agentprimordia/pkg"
)

func main() {
	provider, err := ap.NewOpenAIProvider(ap.Config{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  "gpt-4o",
	})
	if err != nil {
		log.Fatalf("创建 Provider 失败: %v", err)
	}

	// 加载本地知识库
	knowledgeDir := "./knowledge"
	docs, err := loadKnowledge(knowledgeDir)
	if err != nil {
		log.Fatalf("加载知识库失败: %v", err)
	}
	fmt.Printf("已加载 %d 个知识库文档\n", len(docs))

	// 构造 RAG 的配置
	ragConfig := ap.RAGConfig{
		Provider: newSimpleRAG(docs),
		Mode:     ap.RAGModeAuto, // 每一轮都检索
		TopK:     3,
		MinScore: 0.3,
	}

	agent, err := ap.NewAgent("{{.ProjectName}}", "你是一个基于知识库回答问题的助手", provider,
		ap.WithMaxTurns(10),
		ap.WithRAG(ragConfig),
	)
	if err != nil {
		log.Fatalf("创建 Agent 失败: %v", err)
	}

	// 用户提问
	question := "知识库里有什么内容?"
	fmt.Printf("\n问题: %s\n", question)

	resp, err := agent.Run(context.Background(),
		ap.UserMessage(question))
	if err != nil {
		log.Fatalf("调用失败: %v", err)
	}
	fmt.Printf("回复: %s\n", resp.Content)
}

func loadKnowledge(dir string) ([]string, error) {
	var docs []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".txt" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		docs = append(docs, string(data))
	}
	return docs, nil
}

// simpleRAG 是个极简的 RAG provider,基于子串匹配。
// 生产环境应替换为向量检索 (VectorStore)。
type simpleRAG struct {
	docs []string
}

func newSimpleRAG(docs []string) *simpleRAG {
	return &simpleRAG{docs: docs}
}

func (r *simpleRAG) Search(ctx context.Context, query string, topK int) ([]*ap.RAGDocument, error) {
	var results []*ap.RAGDocument
	for i, doc := range r.docs {
		// 简化版：文档 ID 用索引
		results = append(results, &ap.RAGDocument{
			ID:      fmt.Sprintf("doc-%d", i),
			Content: doc,
			Score:   1.0,
		})
		if len(results) >= topK {
			break
		}
	}
	return results, nil
}
