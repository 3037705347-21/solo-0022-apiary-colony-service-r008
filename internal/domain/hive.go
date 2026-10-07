package domain

import "time"

type HiveState string

const (
	HiveColonized  HiveState = "colonized"
	HiveProduction HiveState = "production"
	HiveWintering  HiveState = "wintering"
	HiveRetired    HiveState = "retired"
)

type HiveModel string

const (
	ModelLangstroth HiveModel = "langstroth"
	ModelTopBar     HiveModel = "top_bar"
	ModelWarre      HiveModel = "warre"
)

// Hive 表示一只蜂箱及其当前蜂群状态。
type Hive struct {
	ID               string     `json:"id"`
	ApiaryID         string     `json:"apiaryId"`
	Model            HiveModel  `json:"model"`
	State            HiveState  `json:"state"`
	InstalledAt      time.Time  `json:"installedAt"`
	ActiveQueenID    string     `json:"activeQueenId"`
	LastInspectionAt *time.Time `json:"lastInspectionAt"`
}

// Transition 描述一次成功的蜂群状态迁移。
type Transition struct {
	HiveID string    `json:"hiveId"`
	From   HiveState `json:"from"`
	To     HiveState `json:"to"`
}

var hiveTransitions = map[HiveState]map[HiveState]bool{
	HiveColonized:  {HiveProduction: true, HiveRetired: true},
	HiveProduction: {HiveWintering: true, HiveRetired: true},
	HiveWintering:  {HiveProduction: true, HiveRetired: true},
	HiveRetired:    {},
}

func NewHive(id, apiaryID string, model HiveModel, installedAt time.Time) (Hive, error) {
	if err := ValidateIdentifier("id", id, 3, 40); err != nil {
		return Hive{}, err
	}
	if err := ValidateIdentifier("apiaryId", apiaryID, 3, 40); err != nil {
		return Hive{}, err
	}
	if !ValidHiveModel(model) {
		return Hive{}, Invalid("model", "model 必须是 %s、%s 或 %s", ModelLangstroth, ModelTopBar, ModelWarre)
	}
	return Hive{
		ID:          TrimText(id),
		ApiaryID:    TrimText(apiaryID),
		Model:       model,
		State:       HiveColonized,
		InstalledAt: installedAt.UTC(),
	}, nil
}

func ValidHiveModel(model HiveModel) bool {
	switch model {
	case ModelLangstroth, ModelTopBar, ModelWarre:
		return true
	default:
		return false
	}
}

func ValidHiveState(state HiveState) bool {
	switch state {
	case HiveColonized, HiveProduction, HiveWintering, HiveRetired:
		return true
	default:
		return false
	}
}

// CanTransitionHive 判断 from 到 to 是否为合法迁移，迁移到自身始终非法。
func CanTransitionHive(from, to HiveState) bool {
	targets, ok := hiveTransitions[from]
	if !ok {
		return false
	}
	return targets[to]
}

func (h Hive) AcceptsInspection() bool {
	return h.State != HiveRetired
}

func (h Hive) AcceptsQueenChange() bool {
	return h.State != HiveRetired
}

// HiveGate 描述一次已提交状态对外允许的写入范围。
type HiveGate struct {
	State       HiveState `json:"state"`
	Revision    int64     `json:"revision"`
	Writable    bool      `json:"writable"`
	PublishedAt time.Time `json:"publishedAt"`
}

// GateFor 按蜂群状态推导写入门闸。退休状态不再接受巡检与换王。
func GateFor(state HiveState, at time.Time) HiveGate {
	return HiveGate{
		State:       state,
		Writable:    state != HiveRetired,
		PublishedAt: at.UTC(),
	}
}

// HiveAdvancePlan 是一次待提交的状态推进：它记录规划时观察到的来源状态
// 与版本，提交阶段按这份计划写入目标状态。
type HiveAdvancePlan struct {
	HiveID   string
	From     HiveState
	To       HiveState
	Revision int64
}

// PlanHiveAdvance 依据规划时读到的蜂箱状态与版本推导一次状态推进。
func PlanHiveAdvance(hiveID string, from HiveState, revision int64, target HiveState) (HiveAdvancePlan, error) {
	if !ValidHiveState(target) {
		return HiveAdvancePlan{}, Invalid(
			"target",
			"target 必须是 %s、%s、%s 或 %s",
			HiveColonized, HiveProduction, HiveWintering, HiveRetired,
		)
	}
	// 推进到当前状态不构成迁移，按状态冲突拒绝，避免误操作被记成一次状态变更。
	if from == target {
		return HiveAdvancePlan{}, InvalidState(
			"蜂箱 %q 当前已处于 %q 状态，推进到同一状态不构成迁移", hiveID, from)
	}
	if !CanTransitionHive(from, target) {
		return HiveAdvancePlan{}, InvalidState(
			"蜂箱 %q 不允许从 %q 迁移到 %q", hiveID, from, target)
	}
	return HiveAdvancePlan{HiveID: hiveID, From: from, To: target, Revision: revision}, nil
}
