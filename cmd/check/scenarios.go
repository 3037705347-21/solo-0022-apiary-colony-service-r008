package main

import (
	"fmt"
	"strings"
	"sync"
)

const (
	demoApiaryID = "apiary-north"
	demoHiveID   = "hive-01"
)

func seedApiary(c *client, id, name, district, keeper string) error {
	_, err := c.expect("POST", "/apiaries", map[string]any{
		"id":       id,
		"name":     name,
		"district": district,
		"keeper":   keeper,
	}, 201, `"id":"`+id+`"`)
	return err
}

func seedHive(c *client, apiaryID, hiveID, model string) error {
	_, err := c.expect("POST", "/apiaries/"+apiaryID+"/hives", map[string]any{
		"id":    hiveID,
		"model": model,
	}, 201, `"state":"colonized"`)
	return err
}

func seedDefault(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	return seedHive(c, demoApiaryID, demoHiveID, "langstroth")
}

func checkRegisterApiary(c *client) error {
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-north", "name": "北岭蜂场", "district": "北岭片区", "keeper": "周岚",
	}, 201, `"id":"apiary-north"`, `"name":"北岭蜂场"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north", nil, 200,
		`"district":"北岭片区"`, `"keeper":"周岚"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-north", "name": "重复蜂场", "district": "北岭片区", "keeper": "周岚",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries", map[string]any{
		"id": "apiary-south", "name": "", "district": "南坡片区", "keeper": "李澈",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

func checkHiveInstall(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-01", "model": "langstroth",
	}, 201, `"state":"colonized"`, `"apiaryId":"apiary-north"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01", nil, 200, `"model":"langstroth"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-01", "model": "warre",
	}, 409, `"code":"conflict"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-missing/hives", map[string]any{
		"id": "hive-09", "model": "warre",
	}, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/apiaries/apiary-north/hives", map[string]any{
		"id": "hive-02", "model": "dadant",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

func checkRecordInspection(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 9, "broodPattern": "patchy",
		"temper": "calm", "miteCount": 3, "note": "",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 3, "broodPattern": "patchy",
		"temper": "aggressive", "miteCount": 12, "note": "西侧巢门观察",
	}, 201, `"inspection-000001"`, `"mite_load"`, `"temper_risk"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/inspections", nil, 200, `"note":"西侧巢门观察"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-11T09:00:00Z", "colonyStrength": 3, "broodPattern": "solid",
		"temper": "calm", "miteCount": 1, "note": "",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}
	return nil
}

func checkReplaceQueen(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-04-02T08:00:00Z", "reason": "aging",
	}, 201, `"queen-000001"`, `"markColor":"white"`, `"state":"laying"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "cells", "introducedAt": "2026-04-20T08:00:00Z", "reason": "aggression",
	}, 201, `"queen-000002"`, `"markColor":"white"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/hive-01/queens", nil, 200,
		`"queen-000001"`, `"superseded"`, `"queen-000002"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "imported", "introducedAt": "2026-05-01T08:00:00Z", "reason": "aging",
	}, 400, `"code":"invalid_request"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/queens", map[string]any{
		"origin": "bred", "introducedAt": "2026-05-02T08:00:00Z", "reason": "loss",
	}, 409, `"code":"invalid_state"`); err != nil {
		return err
	}
	return nil
}

func checkAdvanceHiveState(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 200,
		`"from":"colonized"`, `"to":"production"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "colonized"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/state", map[string]any{"target": "production"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-missing/state", map[string]any{"target": "production"}, 404,
		`"code":"not_found"`); err != nil {
		return err
	}
	return nil
}

func checkReadApiaryStatus(c *client) error {
	if err := seedApiary(c, demoApiaryID, "北岭蜂场", "北岭片区", "周岚"); err != nil {
		return err
	}
	for _, hiveID := range []string{"hive-01", "hive-02", "hive-03"} {
		if err := seedHive(c, demoApiaryID, hiveID, "langstroth"); err != nil {
			return err
		}
	}
	if _, err := c.expect("POST", "/hives/hive-02/state", map[string]any{"target": "production"}, 200,
		`"to":"production"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/hive-01/inspections", map[string]any{
		"observedAt": "2026-04-10T09:00:00Z", "colonyStrength": 4, "broodPattern": "solid",
		"temper": "calm", "miteCount": 12, "note": "",
	}, 201, `"mite_load"`); err != nil {
		return err
	}
	report, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=2026-04-12T00:00:00Z", nil, 200,
		`"hiveCount":3`, `"colonized":2`, `"production":1`, `"layingQueens":0`)
	if err != nil {
		return err
	}
	for _, fragment := range []string{
		`"needsInspection":["hive-02","hive-03"]`,
		`"concernHives":["hive-01"]`,
	} {
		if !strings.Contains(report.text(), fragment) {
			return fmt.Errorf("状态汇总缺少 %q：%s", fragment, report.text())
		}
	}
	if err := report.mustOrder(`"hiveId":"hive-01"`, `"hiveId":"hive-02"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-missing/status", nil, 404, `"code":"not_found"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/apiaries/apiary-north/status?asOf=not-a-time", nil, 400,
		`"code":"invalid_request"`); err != nil {
		return err
	}
	return nil
}

// checkAdvanceConcurrency 走真实 HTTP 入口验证同一只蜂箱上并发状态推进的
// 写入门禁：同一目标的并发推进至多一次成功，其余拿到明确的冲突结果，
// 失败的一次重试也不会把蜂箱再往前推。
func checkAdvanceConcurrency(c *client) error {
	if err := seedDefault(c); err != nil {
		return err
	}

	const concurrency = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]*response, concurrency)
	errs := make([]error, concurrency)
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], errs[index] = c.call("POST", "/hives/"+demoHiveID+"/state",
				map[string]any{"target": "production"})
		}(i)
	}
	close(start)
	wg.Wait()

	success, conflict := 0, 0
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("并发推进请求 %d 出错：%w", i, err)
		}
		// 两种失败结果都是明确的冲突类拒绝，HTTP 均为 409：
		//   - conflict：规划时读到的版本在提交前已被胜出者推进（乐观版本失效）；
		//   - invalid_state：规划时胜出者已生效，本请求等同于推进到当前状态。
		// 无论哪种，调用方都能与唯一的 200 成功结果明确区分。
		switch results[i].status {
		case 200:
			if err := results[i].expect(200, `"from":"colonized"`, `"to":"production"`); err != nil {
				return fmt.Errorf("并发推进请求 %d 成功响应异常：%w", i, err)
			}
			success++
		case 409:
			text := results[i].text()
			if !strings.Contains(text, `"code":"conflict"`) &&
				!strings.Contains(text, `"code":"invalid_state"`) {
				return fmt.Errorf("并发推进请求 %d 冲突响应缺少预期错误码：%s", i, text)
			}
			conflict++
		default:
			return fmt.Errorf("并发推进请求 %d 得到未预期状态 %d：%s", i, results[i].status, results[i].text())
		}
	}
	if success != 1 || conflict != concurrency-1 {
		return fmt.Errorf("并发推进结果不符合预期：成功 %d 次、冲突 %d 次（要求成功恰为 1 次，其余全部为 409）", success, conflict)
	}

	// 状态只前进一次：当前应为 production，重复推进到 production 必须被拒绝。
	if _, err := c.expect("POST", "/hives/"+demoHiveID+"/state", map[string]any{"target": "production"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	// 冲突请求重新读取后按当前状态规划：production 不能回到 colonized，
	// 合法的下一步仍可成功，证明冲突没有把状态机推乱。
	if _, err := c.expect("POST", "/hives/"+demoHiveID+"/state", map[string]any{"target": "colonized"}, 409,
		`"code":"invalid_state"`); err != nil {
		return err
	}
	if _, err := c.expect("POST", "/hives/"+demoHiveID+"/state", map[string]any{"target": "wintering"}, 200,
		`"from":"production"`, `"to":"wintering"`); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/"+demoHiveID, nil, 200, `"state":"wintering"`); err != nil {
		return err
	}

	// 退役与随后写入并发交错：退役成功后立刻到达的巡检与换王都必须被拒绝，
	// 且不产生记录、不刷新最近巡检时间。
	retired := make(chan struct{})
	postRetireErrs := make([]error, 2)
	var writeWG sync.WaitGroup
	writeWG.Add(2)
	go func() {
		defer writeWG.Done()
		<-retired
		_, postRetireErrs[0] = c.call("POST", "/hives/"+demoHiveID+"/inspections", map[string]any{
			"observedAt": "2026-04-11T09:00:00Z", "colonyStrength": 3, "broodPattern": "solid",
			"temper": "calm", "miteCount": 1, "note": "",
		})
	}()
	go func() {
		defer writeWG.Done()
		<-retired
		_, postRetireErrs[1] = c.call("POST", "/hives/"+demoHiveID+"/queens", map[string]any{
			"origin": "bred", "introducedAt": "2026-05-02T08:00:00Z", "reason": "loss",
		})
	}()
	if _, err := c.expect("POST", "/hives/"+demoHiveID+"/state", map[string]any{"target": "retired"}, 200,
		`"to":"retired"`); err != nil {
		return err
	}
	close(retired)
	writeWG.Wait()
	for i, err := range postRetireErrs {
		if err != nil {
			return fmt.Errorf("退役后写入请求 %d 出错：%w", i, err)
		}
	}
	if _, err := c.expect("GET", "/hives/"+demoHiveID+"/inspections", nil, 200, "[]"); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/"+demoHiveID+"/queens", nil, 200, "[]"); err != nil {
		return err
	}
	if _, err := c.expect("GET", "/hives/"+demoHiveID, nil, 200, `"lastInspectionAt":null`); err != nil {
		return err
	}
	return nil
}
