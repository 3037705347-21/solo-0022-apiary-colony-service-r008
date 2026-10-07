package store

import (
	"fmt"
	"sync"
	"time"

	"example.com/apiary-colony-service/internal/domain"
)

// Memory 是并发安全的内存仓储。所有写入都在写锁内完成状态校验，
// 读取则通过 Snapshot 得到一致视图。
type Memory struct {
	mu sync.RWMutex

	apiaries    map[string]domain.Apiary
	hives       map[string]domain.Hive
	inspections map[string][]domain.Inspection
	queens      map[string][]domain.Queen

	hiveRevisions map[string]int64
	hiveGates     map[string]domain.HiveGate

	inspectionSeq int
	queenSeq      int
}

func NewMemory() *Memory {
	return &Memory{
		apiaries:      make(map[string]domain.Apiary),
		hives:         make(map[string]domain.Hive),
		inspections:   make(map[string][]domain.Inspection),
		queens:        make(map[string][]domain.Queen),
		hiveRevisions: make(map[string]int64),
		hiveGates:     make(map[string]domain.HiveGate),
	}
}

func (m *Memory) CreateApiary(apiary domain.Apiary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apiaries[apiary.ID]; ok {
		return domain.Conflict("蜂场 %q 已存在", apiary.ID)
	}
	m.apiaries[apiary.ID] = apiary
	return nil
}

func (m *Memory) GetApiary(id string) (domain.Apiary, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	apiary, ok := m.apiaries[id]
	return apiary, ok
}

func (m *Memory) CreateHive(hive domain.Hive) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apiaries[hive.ApiaryID]; !ok {
		return domain.NotFound("蜂场", hive.ApiaryID)
	}
	if _, ok := m.hives[hive.ID]; ok {
		return domain.Conflict("蜂箱 %q 已存在", hive.ID)
	}
	m.hives[hive.ID] = hive
	return nil
}

func (m *Memory) GetHive(id string) (domain.Hive, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hive, ok := m.hives[id]
	return hive, ok
}

// HiveCommitView 返回提交阶段需要的最新状态与版本，供推进规划使用。
func (m *Memory) HiveCommitView(hiveID string) (domain.HiveState, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return "", 0, domain.NotFound("蜂箱", hiveID)
	}
	return hive.State, m.hiveRevisions[hiveID], nil
}

// CommitHiveAdvance 按规划结果写入目标状态并递增状态版本。它按规划阶段
// 给出的来源状态判定合法性，并发规划可以依次提交同一目标。
func (m *Memory) CommitHiveAdvance(plan domain.HiveAdvancePlan) (domain.Transition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[plan.HiveID]
	if !ok {
		return domain.Transition{}, domain.NotFound("蜂箱", plan.HiveID)
	}
	if !domain.CanTransitionHive(plan.From, plan.To) && plan.From != plan.To {
		return domain.Transition{}, domain.InvalidState(
			"蜂箱 %q 不允许从 %q 迁移到 %q", plan.HiveID, plan.From, plan.To)
	}
	hive.State = plan.To
	m.hives[plan.HiveID] = hive
	m.hiveRevisions[plan.HiveID] = plan.Revision + 1
	return domain.Transition{HiveID: plan.HiveID, From: plan.From, To: plan.To}, nil
}

// SetHiveGate 发布蜂箱的写入门闸，供巡检与换王的准入判定读取。
func (m *Memory) SetHiveGate(hiveID string, gate domain.HiveGate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.hives[hiveID]; !ok {
		return domain.NotFound("蜂箱", hiveID)
	}
	m.hiveGates[hiveID] = gate
	return nil
}

// acceptsWriteLocked 返回当前写入门闸是否允许巡检与换王写入。
func (m *Memory) acceptsWriteLocked(hiveID string, hive domain.Hive) bool {
	if gate, ok := m.hiveGates[hiveID]; ok {
		return gate.Writable
	}
	return hive.State != domain.HiveRetired
}

// AddInspection 分配巡检编号、写入记录，并推进蜂箱的最近巡检时间。
// 蜂箱存在与可接受巡检的判定与写入处于同一把锁，避免退役与巡检之间的竞态。
func (m *Memory) AddInspection(hiveID string, inspection domain.Inspection) (domain.Inspection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Inspection{}, domain.NotFound("蜂箱", hiveID)
	}
	if !m.acceptsWriteLocked(hiveID, hive) {
		return domain.Inspection{}, domain.InvalidState("蜂箱 %q 处于 %q 状态，不接受巡检", hiveID, hive.State)
	}
	m.inspectionSeq++
	inspection.ID = fmt.Sprintf("inspection-%06d", m.inspectionSeq)
	inspection.HiveID = hiveID
	m.inspections[hiveID] = append(m.inspections[hiveID], inspection)

	observedAt := inspection.ObservedAt
	hive.LastInspectionAt = &observedAt
	m.hives[hiveID] = hive
	return inspection, nil
}

// AddQueen 关闭原有产卵蜂王并写入新蜂王，两件事在同一把锁内完成，
// 保证任意时刻同一蜂箱至多只有一只 laying 蜂王。
func (m *Memory) AddQueen(
	hiveID string,
	queen domain.Queen,
	closeQueenID string,
	closeState domain.QueenState,
	now time.Time,
) (domain.Queen, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hive, ok := m.hives[hiveID]
	if !ok {
		return domain.Queen{}, domain.NotFound("蜂箱", hiveID)
	}
	if !m.acceptsWriteLocked(hiveID, hive) {
		return domain.Queen{}, domain.InvalidState("蜂箱 %q 处于 %q 状态，不接受换王", hiveID, hive.State)
	}
	if closeQueenID != "" {
		if err := m.closeQueenLocked(hiveID, closeQueenID, closeState, now); err != nil {
			return domain.Queen{}, err
		}
	}
	m.queenSeq++
	queen.ID = fmt.Sprintf("queen-%06d", m.queenSeq)
	queen.HiveID = hiveID
	queen.State = domain.QueenLaying
	m.queens[hiveID] = append(m.queens[hiveID], queen)

	hive.ActiveQueenID = queen.ID
	m.hives[hiveID] = hive
	return queen, nil
}

func (m *Memory) closeQueenLocked(hiveID, queenID string, state domain.QueenState, now time.Time) error {
	items := m.queens[hiveID]
	for index, item := range items {
		if item.ID != queenID {
			continue
		}
		if !item.IsLaying() {
			return domain.Conflict("蜂王 %q 已经关闭", queenID)
		}
		closedAt := now.UTC()
		item.State = state
		item.ClosedAt = &closedAt
		items[index] = item
		m.queens[hiveID] = items
		return nil
	}
	return domain.Storage("蜂箱 %q 上的蜂王 %q 记录缺失", hiveID, queenID)
}

func (m *Memory) ListInspections(hiveID string) ([]domain.Inspection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.inspections[hiveID]
	out := make([]domain.Inspection, len(items))
	copy(out, items)
	return out, nil
}

func (m *Memory) ListQueens(hiveID string) ([]domain.Queen, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.hives[hiveID]; !ok {
		return nil, domain.NotFound("蜂箱", hiveID)
	}
	items := m.queens[hiveID]
	out := make([]domain.Queen, len(items))
	copy(out, items)
	return out, nil
}

// Snapshot 返回深拷贝的一致视图，供汇总类读取使用。
func (m *Memory) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := Snapshot{
		Apiaries:          make(map[string]domain.Apiary, len(m.apiaries)),
		Hives:             make(map[string]domain.Hive, len(m.hives)),
		InspectionsByHive: make(map[string][]domain.Inspection, len(m.inspections)),
		QueensByHive:      make(map[string][]domain.Queen, len(m.queens)),
	}
	for id, apiary := range m.apiaries {
		snapshot.Apiaries[id] = apiary
	}
	for id, hive := range m.hives {
		snapshot.Hives[id] = cloneHive(hive)
	}
	for id, items := range m.inspections {
		copied := make([]domain.Inspection, len(items))
		copy(copied, items)
		for index := range copied {
			copied[index].Concerns = append([]string{}, items[index].Concerns...)
		}
		snapshot.InspectionsByHive[id] = copied
	}
	for id, items := range m.queens {
		copied := make([]domain.Queen, len(items))
		copy(copied, items)
		for index := range copied {
			if items[index].ClosedAt != nil {
				closedAt := *items[index].ClosedAt
				copied[index].ClosedAt = &closedAt
			}
		}
		snapshot.QueensByHive[id] = copied
	}
	return snapshot
}

func cloneHive(hive domain.Hive) domain.Hive {
	if hive.LastInspectionAt != nil {
		observedAt := *hive.LastInspectionAt
		hive.LastInspectionAt = &observedAt
	}
	return hive
}
