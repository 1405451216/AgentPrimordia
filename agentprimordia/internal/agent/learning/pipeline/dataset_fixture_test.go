// dataset_fixture_test.go — ap-dataset-v1 跨语言对账夹具（Go 为权威生成方）
//
// 产出 testdata/dataset_fixture.json：{manifest, jsonl, report}——
// TS 消费端（sdk/typescript/src/learning/pipeline.ts）读同一文件做
// 解析/互证/判据复算对账（矩阵 #2：TS 以工件消费者身份对等）。
// 再生方式：AP_WRITE_PIPELINE_FIXTURE=1 go test ./internal/agent/learning/pipeline/ -run TestWritePipelineFixture
package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type datasetFixture struct {
	Manifest DatasetManifest `json:"manifest"`
	JSONL    string          `json:"jsonl"`
	Report   ShadowReport    `json:"report"`
}

func buildDatasetFixture(t *testing.T) *datasetFixture {
	t.Helper()
	trajs := buildTrajectories(3, 0)
	cands, _ := Curate(trajs, "tool_selection", CuratorConfig{})
	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	ds, err := Export(cands, "tool_selection", "test-suite", at)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyDataset(ds); err != nil {
		t.Fatal(err)
	}
	// 报告夹具：4/5 配对（champion 5/5、shadow 4/5 的确定性形态——判据应不过）
	rep := ShadowReport{
		ManifestID:      ds.Manifest.ManifestID,
		ChampionModel:   "flagship-x",
		ShadowModel:     "distilled-8b-v1",
		N:               5,
		ChampionSuccess: 5,
		ShadowSuccess:   4,
		ChampionRate:    1,
		ShadowRate:      0.8,
		CreatedAt:       at,
	}
	lo, _ := wilsonInterval(rep.ShadowSuccess, rep.N)
	rep.ShadowWilsonLo = lo
	rep.Ratio = rep.ShadowRate / rep.ChampionRate
	rep.RatioLower = lo / rep.ChampionRate
	rep.McNemarP = mcnemarExactP(0, 1)
	rep.Passed = rep.Ratio >= 0.85 && rep.RatioLower >= 0.80
	return &datasetFixture{Manifest: ds.Manifest, JSONL: string(ds.JSONL), Report: rep}
}

// TestWritePipelineFixture 生成夹具（默认跳过）。
func TestWritePipelineFixture(t *testing.T) {
	if os.Getenv("AP_WRITE_PIPELINE_FIXTURE") == "" {
		t.Skip("设置 AP_WRITE_PIPELINE_FIXTURE=1 以重新生成夹具")
	}
	data, err := json.MarshalIndent(buildDatasetFixture(t), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("testdata", "dataset_fixture.json"), append(data, byte(10)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log("数据集夹具已写出")
}

// TestPipelineFixtureGolden 黄金门：夹具与当前实现一致（防契约漂移）。
func TestPipelineFixtureGolden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "dataset_fixture.json"))
	if err != nil {
		t.Skipf("夹具文件不存在: %v", err)
	}
	var golden datasetFixture
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	cur := buildDatasetFixture(t)
	if golden.JSONL != cur.JSONL || golden.Manifest != cur.Manifest {
		t.Fatal("数据集契约漂移：夹具与当前实现不一致（AP_WRITE_PIPELINE_FIXTURE=1 重新生成前先评审）")
	}
	// 报告字段逐项复算一致
	if golden.Report.ShadowWilsonLo != cur.Report.ShadowWilsonLo ||
		golden.Report.Ratio != cur.Report.Ratio ||
		golden.Report.RatioLower != cur.Report.RatioLower ||
		golden.Report.Passed != cur.Report.Passed {
		t.Fatal("影子报告统计漂移")
	}
}
