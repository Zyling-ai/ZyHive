// pkg/tools/skillopt_tool.go — SkillOpt self-evolution tools.
//
// Let an agent log its own predictions and backfill outcomes for an evolving
// skill, feeding the predict → oracle → critic → evolve loop:
//   - skillopt_predict({skillId, prediction, contextDigest?}) — record a prediction
//   - skillopt_oracle({skillId, entryId, result, hit})        — backfill the outcome
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Zyling-ai/zyhive/pkg/llm"
	"github.com/Zyling-ai/zyhive/pkg/skillopt"
)

var skilloptPredictDef = llm.ToolDef{
	Name: "skillopt_predict",
	Description: "为某个可自我进化的技能记录一条**预测**（待真实结果出来后用 skillopt_oracle 回填）。" +
		"当你基于某技能做出可被事实检验的判断时调用，例如比分、涨跌、任务是否达成。返回预测 ID。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"skillId": {"type": "string", "description": "技能 ID（skills/ 下的目录名）"},
			"prediction": {"type": "string", "description": "预测内容（要可被事实检验）"},
			"contextDigest": {"type": "string", "description": "做出该预测时的关键依据/上下文摘要（可选，便于复盘）"}
		},
		"required": ["skillId", "prediction"]
	}`),
}

var skilloptOracleDef = llm.ToolDef{
	Name: "skillopt_oracle",
	Description: "为之前用 skillopt_predict 记录的预测**回填真实结果**，标记命中/未命中。" +
		"真实结果出来后调用，系统会据此自动复盘并进化该技能。",
	InputSchema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"skillId": {"type": "string", "description": "技能 ID"},
			"entryId": {"type": "string", "description": "skillopt_predict 返回的预测 ID"},
			"result": {"type": "string", "description": "真实结果描述"},
			"hit": {"type": "boolean", "description": "预测是否命中：true=命中, false=未命中"}
		},
		"required": ["skillId", "entryId", "result", "hit"]
	}`),
}

func (r *Registry) handleSkilloptPredict(_ context.Context, input json.RawMessage) (string, error) {
	var req struct {
		SkillID       string `json:"skillId"`
		Prediction    string `json:"prediction"`
		ContextDigest string `json:"contextDigest"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	req.SkillID = strings.TrimSpace(req.SkillID)
	req.Prediction = strings.TrimSpace(req.Prediction)
	if req.SkillID == "" || req.Prediction == "" {
		return "", fmt.Errorf("skillId and prediction are required")
	}
	s := skillopt.NewStore(r.workspaceDir, req.SkillID)
	if err := s.Init(); err != nil {
		return "", fmt.Errorf("skillopt init: %w", err)
	}
	entry, err := s.Append(skillopt.LedgerEntry{
		Prediction:    req.Prediction,
		ContextDigest: req.ContextDigest,
		SessionRef:    r.sessionID,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("✅ 已记录预测 %s（技能 %s）。真实结果出来后请用 skillopt_oracle 回填。", entry.ID, req.SkillID), nil
}

func (r *Registry) handleSkilloptOracle(_ context.Context, input json.RawMessage) (string, error) {
	var req struct {
		SkillID string `json:"skillId"`
		EntryID string `json:"entryId"`
		Result  string `json:"result"`
		Hit     *bool  `json:"hit"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(req.SkillID) == "" || strings.TrimSpace(req.EntryID) == "" || req.Hit == nil {
		return "", fmt.Errorf("skillId, entryId and hit are required")
	}
	s := skillopt.NewStore(r.workspaceDir, req.SkillID)
	if err := s.Oracle(req.EntryID, req.Result, *req.Hit); err != nil {
		return "", err
	}
	verdict := "未命中"
	if *req.Hit {
		verdict = "命中"
	}
	return fmt.Sprintf("✅ 已回填 %s：%s。", req.EntryID, verdict), nil
}
