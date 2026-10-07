package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type client struct {
	base string
	http *http.Client
}

type response struct {
	status int
	body   []byte
}

func newClient(base string) *client {
	return &client{base: base, http: &http.Client{Timeout: 5 * time.Second}}
}

func (c *client) call(method, path string, payload any) (*response, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	reply, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer reply.Body.Close()
	raw, err := io.ReadAll(reply.Body)
	if err != nil {
		return nil, err
	}
	return &response{status: reply.StatusCode, body: raw}, nil
}

func (c *client) expect(method, path string, payload any, wantStatus int, wantFragments ...string) (*response, error) {
	reply, err := c.call(method, path, payload)
	if err != nil {
		return nil, err
	}
	if err := reply.expect(wantStatus, wantFragments...); err != nil {
		return nil, fmt.Errorf("%s %s %w", method, path, err)
	}
	return reply, nil
}

func (r *response) expect(wantStatus int, wantFragments ...string) error {
	if r.status != wantStatus {
		return fmt.Errorf("期望状态 %d，实际 %d，响应 %s", wantStatus, r.status, r.text())
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(r.text(), fragment) {
			return fmt.Errorf("响应缺少 %q：%s", fragment, r.text())
		}
	}
	return nil
}

func (r *response) text() string {
	return string(r.body)
}

func (r *response) mustOrder(before, after string) error {
	text := r.text()
	first := strings.Index(text, before)
	second := strings.Index(text, after)
	if first < 0 || second < 0 {
		return fmt.Errorf("响应缺少排序断言片段 %q / %q：%s", before, after, text)
	}
	if first >= second {
		return fmt.Errorf("响应顺序不符合预期，%q 应出现在 %q 之前：%s", before, after, text)
	}
	return nil
}
