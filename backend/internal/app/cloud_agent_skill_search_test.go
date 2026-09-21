package app

import (
	"encoding/json"
	"strings"
	"testing"
)

// skill_search 的契约测试：注册、分词打分、索引兜底、边界与空集。
// 沿用 #577 的同文件断言风格（含 sentinel 泄漏检查）。

func skillSearchTestSkills() []cloudAgentSkill {
	return []cloudAgentSkill{
		{ID: "s1", Name: "consumer-psychology", Description: "当写带货口播、广告脚本、详情页与直播话术，需要设计转化结构时调用。核心能力：价格锚点/比价阻断/社会证据。"},
		{ID: "s2", Name: "shortform-drama-playbook", Description: "当写 3 分钟以内反转短剧的分镜脚本时调用。核心能力：钩子/因果/反转收束。"},
		{ID: "s3", Name: "h3-video-prompt-suite", Description: "当为 H3 视频模型写生成提示词时调用。核心能力：六段式/运镜三段式/负向提示词。"},
	}
}

func TestCloudAgentPolicyRegistersSkillSearchAlongsideReadFile(t *testing.T) {
	req := CloudAgentRequest{SkillIDs: []string{"s1"}}
	names := map[string]bool{}
	for _, tool := range cloudAgentTools(req) {
		fn, ok := tool["function"].(map[string]any)
		if !ok {
			t.Fatalf("tool without function: %v", tool)
		}
		names[fn["name"].(string)] = true
	}
	if !names["skill_read_file"] || !names["skill_search"] {
		t.Fatalf("skill tools not registered together: %v", names)
	}
	// 无技能时两个工具都不该出现
	for _, tool := range cloudAgentTools(CloudAgentRequest{}) {
		fn := tool["function"].(map[string]any)
		if fn["name"] == "skill_search" || fn["name"] == "skill_read_file" {
			t.Fatalf("skill tool leaked without skills: %v", fn["name"])
		}
	}
}

func TestCloudAgentSkillSearchRanksNameHitsAboveDescriptionHits(t *testing.T) {
	result, err := cloudAgentSearchSkills(skillSearchTestSkills(), "口播 转化", 0)
	if err != nil {
		t.Fatal(err)
	}
	matches := result["matches"].([]map[string]any)
	if len(matches) == 0 {
		t.Fatal("no matches for 口播/转化")
	}
	if matches[0]["skillId"] != "s1" {
		t.Fatalf("expected consumer-psychology first, got %v", matches[0]["skillId"])
	}
	if matches[0]["path"] != cloudAgentSkillEntryPath {
		t.Fatalf("expected entry path, got %v", matches[0]["path"])
	}
	if matches[0]["score"].(int) <= 0 {
		t.Fatalf("expected positive score, got %v", matches[0]["score"])
	}
	if snippet, _ := matches[0]["snippet"].(string); strings.Contains(snippet, "PRIVATE") || len(snippet) > 200 {
		t.Fatalf("snippet unsafe or oversized: %q", snippet)
	}
}

func TestCloudAgentSkillSearchWithoutKeywordListsIndex(t *testing.T) {
	result, err := cloudAgentSearchSkills(skillSearchTestSkills(), "  ", 0)
	if err != nil {
		t.Fatal(err)
	}
	matches := result["matches"].([]map[string]any)
	if len(matches) != 3 {
		t.Fatalf("expected full index of 3, got %d", len(matches))
	}
	if result["total"].(int) != 3 {
		t.Fatalf("expected total 3, got %v", result["total"])
	}
}

func TestCloudAgentSkillSearchClampsLimitAndHandlesNoHit(t *testing.T) {
	skills := skillSearchTestSkills()
	result, err := cloudAgentSearchSkills(skills, "钩子", 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(result["matches"].([]map[string]any)) != 1 {
		t.Fatalf("expected single hit for 钩子, got %v", result["matches"])
	}
	result, err = cloudAgentSearchSkills(skills, "量子纠缠炒菜", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result["matches"].([]map[string]any)) != 0 {
		t.Fatalf("expected no matches, got %v", result["matches"])
	}
	if guidance, _ := result["guidance"].(string); !strings.Contains(guidance, "skill_read_file") {
		t.Fatalf("no-hit guidance must point at skill_read_file: %q", guidance)
	}
}

func TestCloudAgentSkillSearchEmptySkillsReturnsGuidance(t *testing.T) {
	result, err := cloudAgentSearchSkills(nil, "口播", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result["matches"].([]map[string]any)) != 0 {
		t.Fatal("expected empty matches without skills")
	}
	if guidance, _ := result["guidance"].(string); !strings.Contains(guidance, "没有已启用") {
		t.Fatalf("guidance mismatch: %q", guidance)
	}
}

func TestCloudAgentSkillSearchNeverInlinesSkillBody(t *testing.T) {
	skills := []cloudAgentSkill{{ID: "s1", Name: "demo", Description: "PRIVATE_SKILL_BODY_SENTINEL 只在正文，不在描述", Instruction: "PRIVATE_INSTRUCTION_SENTINEL"}}
	result, err := cloudAgentSearchSkills(skills, "demo", 0)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "PRIVATE_INSTRUCTION_SENTINEL") {
		t.Fatal("skill_search leaked instruction body")
	}
}
