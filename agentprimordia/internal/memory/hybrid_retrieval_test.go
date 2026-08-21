// hybrid_retrieval_test.go — v5.3 混合检索路由 + 记忆迁移测试
package memory

import "testing"

func TestClassifyQuery(t *testing.T) {
	cases := []struct {
		q    string
		want QueryKind
	}{
		{"查找 user_id=42 的记录", QueryKeyword},
		{"如何优化数据库性能", QuerySemantic},
		{"user_42 的超时问题如何解决", QueryHybrid},
	}
	for _, c := range cases {
		if got := ClassifyQuery(c.q); got != c.want {
			t.Errorf("ClassifyQuery(%q) = %v, 期望 %v", c.q, got, c.want)
		}
	}
}

func TestHybridRetrieverKeywordRoute(t *testing.T) {
	r := NewHybridRetriever()
	r.Add(HybridDoc{ID: "1", Text: "config user_id 42 timeout setting"})
	r.Add(HybridDoc{ID: "2", Text: "how to improve similar database performance"})

	docs := r.Retrieve("user_id 42", 1)
	if len(docs) != 1 || docs[0].ID != "1" {
		t.Errorf("精确查询应命中文档 1，得到 %+v", docs)
	}
}

func TestHybridRetrieverSemanticRoute(t *testing.T) {
	r := NewHybridRetriever()
	r.Add(HybridDoc{ID: "kw", Text: "error code 500"})
	r.Add(HybridDoc{ID: "sem", Text: "数据库性能优化指南", Vector: textPseudoVec("如何优化慢查询提升数据库性能")})

	docs := r.Retrieve("如何提升慢查询的数据库性能", 1)
	if len(docs) != 1 || docs[0].ID != "sem" {
		t.Errorf("语义查询应命中向量通道文档，得到 %+v", docs)
	}
}

func TestTransferIndexRecall(t *testing.T) {
	idx := NewTransferIndex()
	idx.Record("deploy k8s cluster rollout stuck image pull", "先预拉镜像再延长超时", 5)
	idx.Record("write unit tests for parser", "用表驱动覆盖错误分支", 2)

	hits := idx.Recall("部署 k8s 集群时 rollout 卡在 image pull", 0.2, 3)
	if len(hits) == 0 || !hrContains(hits, "先预拉镜像再延长超时") {
		t.Errorf("相似任务应召回经验，得到 %+v", hits)
	}
	if none := idx.Recall("cooking pasta recipe", 0.2, 3); len(none) != 0 {
		t.Errorf("无关任务不应召回: %+v", none)
	}
}

func hrContains(entries []TransferEntry, exp string) bool {
	for _, e := range entries {
		if e.Experience == exp {
			return true
		}
	}
	return false
}
