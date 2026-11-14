package parser

import (
	"encoding/json"
	"regexp"
	"strings"
)

// 列表页解析结果
type ListPage struct {
	Entries    []ListEntry
	EndMarked  bool   // 是否携带样本站点结束标志且 data-end=true
	Marker     string // 页面声明的标志值
	NextCursor string
}

type ListEntry struct {
	Ref         string `json:"ref"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	PublishTime string `json:"publish_time"`
}

// 详情页解析结果（半份内容时 Complete=false，Fields 仍尽量保留已取得部分）
type Detail struct {
	DocType       string        `json:"doc_type"`
	ProjectNo     string        `json:"project_no"`
	Title         string        `json:"title"`
	PublishTime   string        `json:"publish_time"`
	TargetRef     string        `json:"target_ref"`
	CorrectionNo  string        `json:"correction_no"`
	ScopeAll      bool          `json:"scope_all"`
	ScopePackages []string      `json:"scope_packages"`
	Packages      []Pkg         `json:"packages"`
	Fields        []ParsedField `json:"fields"`
	Complete      bool          `json:"complete"`
}

type Pkg struct {
	No    string `json:"no"`
	Title string `json:"title"`
}

type ParsedField struct {
	Field   string `json:"field"`
	Package string `json:"package"`
	Locator string `json:"locator"`
	Value   string `json:"value"`
	Excerpt string `json:"excerpt"`
}

var (
	reEndMarker   = regexp.MustCompile(`id="end-marker"[^>]*data-marker="([^"]*)"[^>]*data-end="(true|false)"`)
	reEntry       = regexp.MustCompile(`(?s)data-sample-entry[^>]*data-ref="([^"]+)"[^>]*data-publish="([^"]*)"[^>]*>.*?href="([^"]+)"[^>]*>(.*?)</a>`)
	reNextCursor  = regexp.MustCompile(`data-sample-next-cursor="([^"]*)"`)
	reAttr        = regexp.MustCompile(`%s="([^"]*)"`)
	reDocType     = regexp.MustCompile(`data-sample-doc-type="([^"]+)"`)
	reProjectNo   = regexp.MustCompile(`data-sample-project-no="([^"]+)"`)
	reTargetRef   = regexp.MustCompile(`data-sample-target-ref="([^"]+)"`)
	reCorrectionNo = regexp.MustCompile(`data-sample-correction-no="([^"]+)"`)
	reScopeAll    = regexp.MustCompile(`data-sample-scope-all="true"`)
	reScopePkg    = regexp.MustCompile(`data-sample-scope-package="([^"]+)"`)
	rePkg         = regexp.MustCompile(`data-sample-package="([^"]+)"[^>]*data-title="([^"]*)"`)
	reField       = regexp.MustCompile(`(?s)data-sample-field="([^"]+)"[^>]*data-package="([^"]*)"[^>]*data-locator="([^"]*)">(.*?)</div>`)
	reFieldValue  = regexp.MustCompile(`class="field-value">(.*?)</span>`)
	reFieldExcerpt = regexp.MustCompile(`class="field-excerpt">(.*?)</p>`)
	rePublish     = regexp.MustCompile(`data-sample-publish-time="([^"]+)"`)
	reTitle       = regexp.MustCompile(`(?s)<h1 class="notice-title">(.*?)</h1>`)
	reComplete    = regexp.MustCompile(`data-sample-content-complete="true"`)
	reTag         = regexp.MustCompile(`<[^>]+>`)
)

func unescape(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	return strings.TrimSpace(s)
}

// ParseList 解析样本列表页；空列表是否到尾取决于结束标志而非条目数。
func ParseList(html string) ListPage {
	out := ListPage{}
	if m := reEndMarker.FindStringSubmatch(html); m != nil {
		out.Marker = unescape(m[1])
		out.EndMarked = m[2] == "true"
	}
	if m := reNextCursor.FindStringSubmatch(html); m != nil {
		out.NextCursor = unescape(m[1])
	}
	for _, m := range reEntry.FindAllStringSubmatch(html, -1) {
		out.Entries = append(out.Entries, ListEntry{
			Ref:         unescape(m[1]),
			PublishTime: unescape(m[2]),
			URL:         unescape(m[3]),
			Title:       unescape(reTag.ReplaceAllString(m[4], "")),
		})
	}
	return out
}

// ParseDetail 解析详情页；返回 Detail、截断标志与错误说明。
// 半份内容不视为解析崩溃：保留已提取字段，同时显式报错，调用方不得据此删除字段。
func ParseDetail(rawHTML string) (Detail, bool, string) {
	d := Detail{ScopePackages: []string{}, Packages: []Pkg{}, Fields: []ParsedField{}}
	if m := reDocType.FindStringSubmatch(rawHTML); m != nil {
		d.DocType = m[1]
	}
	if m := reProjectNo.FindStringSubmatch(rawHTML); m != nil {
		d.ProjectNo = unescape(m[1])
	}
	if m := reTargetRef.FindStringSubmatch(rawHTML); m != nil {
		d.TargetRef = unescape(m[1])
	}
	if m := reCorrectionNo.FindStringSubmatch(rawHTML); m != nil {
		d.CorrectionNo = unescape(m[1])
	}
	if m := rePublish.FindStringSubmatch(rawHTML); m != nil {
		d.PublishTime = unescape(m[1])
	}
	if m := reTitle.FindStringSubmatch(rawHTML); m != nil {
		d.Title = unescape(m[1])
	}
	d.ScopeAll = reScopeAll.MatchString(rawHTML)
	for _, m := range reScopePkg.FindAllStringSubmatch(rawHTML, -1) {
		d.ScopePackages = append(d.ScopePackages, unescape(m[1]))
	}
	for _, m := range rePkg.FindAllStringSubmatch(rawHTML, -1) {
		d.Packages = append(d.Packages, Pkg{No: unescape(m[1]), Title: unescape(m[2])})
	}
	for _, m := range reField.FindAllStringSubmatch(rawHTML, -1) {
		f := ParsedField{Field: unescape(m[1]), Package: unescape(m[2]), Locator: unescape(m[3])}
		inner := m[4]
		if v := reFieldValue.FindStringSubmatch(inner); v != nil {
			f.Value = unescape(v[1])
		}
		if v := reFieldExcerpt.FindStringSubmatch(inner); v != nil {
			f.Excerpt = unescape(v[1])
		}
		d.Fields = append(d.Fields, f)
	}
	d.Complete = reComplete.MatchString(rawHTML)
	if !d.Complete {
		return d, true, "正文不完整：缺少内容完整标记（半份下载），已保留原件与已提取字段，缺失字段不得视为删除"
	}
	if len(d.Fields) == 0 && d.DocType != "cancellation" {
		return d, false, "未能从正文提取任何业务字段"
	}
	return d, false, ""
}

func (d Detail) JSON() []byte {
	b, _ := json.Marshal(d)
	return b
}
