package sampledata

import (
	"fmt"
	"html"
	"strings"
)

// 样本页面固定标记：parser 与 fetcher 依赖。
const (
	MarkerListEnd   = "data-sample-list-end"
	MarkerContentOK = "data-sample-content-complete"
	MarkerProjectNo = "data-sample-project-no"
	MarkerDocType   = "data-sample-doc-type"
	MarkerTargetRef = "data-sample-target-ref"
	MarkerCorrectionNo = "data-sample-correction-no"
	MarkerScopeAll  = "data-sample-scope-all"
	MarkerScopePkg  = "data-sample-scope-package"
	MarkerPackage   = "data-sample-package"
	MarkerField     = "data-sample-field"
	MarkerPublish   = "data-sample-publish-time"
	MarkerListEntry = "data-sample-entry"
)

// RenderList 渲染列表页。end=true 时输出结束标志（空列表只有带结束标志才算到尾）。
func RenderList(entries []ListEntry, endMarker string, end bool, extraQuery string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>sample list</title></head><body>`)
	b.WriteString(fmt.Sprintf(`<div id="end-marker" data-marker="%s" data-end="%t"></div>`, endMarker, end))
	for _, e := range entries {
		b.WriteString(fmt.Sprintf(`<div class="entry" %s data-ref="%s" data-publish="%s">`, MarkerListEntry, html.EscapeString(e.Ref), html.EscapeString(e.PublishTime)))
		b.WriteString(fmt.Sprintf(`<a class="entry-link" href="/notice/%s%s">%s</a>`, html.EscapeString(e.Ref), extraQuery, html.EscapeString(e.Title)))
		b.WriteString(fmt.Sprintf(`<span class="entry-title">%s</span></div>`, html.EscapeString(e.Title)))
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

// RenderDetail 渲染公告详情；n.Complete=false 时只输出半份正文且不含完整标记。
func RenderDetail(n Notice) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>` + html.EscapeString(n.Title) + `</title></head><body>`)
	b.WriteString(`<article class="notice" ` + MarkerDocType + `="` + html.EscapeString(n.DocType) + `">`)
	b.WriteString(`<h1 class="notice-title">` + html.EscapeString(n.Title) + `</h1>`)
	b.WriteString(fmt.Sprintf(`<div %s="%s"></div>`, MarkerPublish, html.EscapeString(n.PublishTime)))
	if n.ProjectNo != "" {
		b.WriteString(fmt.Sprintf(`<div %s="%s"></div>`, MarkerProjectNo, html.EscapeString(n.ProjectNo)))
	} else {
		b.WriteString(`<div class="missing-project-no">本公告暂未标注采购编号</div>`)
	}
	if n.TargetRef != "" {
		b.WriteString(fmt.Sprintf(`<div %s="%s"></div>`, MarkerTargetRef, html.EscapeString(n.TargetRef)))
	}
	if n.CorrectionNo != "" {
		b.WriteString(fmt.Sprintf(`<div %s="%s"></div>`, MarkerCorrectionNo, html.EscapeString(n.CorrectionNo)))
	}
	if n.ScopeAll {
		b.WriteString(fmt.Sprintf(`<div %s="true"></div>`, MarkerScopeAll))
	}
	for _, p := range n.ScopePkgs {
		b.WriteString(fmt.Sprintf(`<div %s="%s"></div>`, MarkerScopePkg, html.EscapeString(p)))
	}
	b.WriteString(`<section class="packages">`)
	for _, p := range n.Packages {
		b.WriteString(fmt.Sprintf(`<div class="package" %s="%s" data-title="%s">标包 %s %s</div>`,
			MarkerPackage, html.EscapeString(p.No), html.EscapeString(p.Title), html.EscapeString(p.No), html.EscapeString(p.Title)))
	}
	b.WriteString(`</section><section class="fields">`)
	for _, f := range n.Fields {
		// 半份内容：最后一个字段输出后正文戛然而止
		b.WriteString(fmt.Sprintf(`<div class="field" %s="%s" data-package="%s" data-locator="%s">`,
			MarkerField, html.EscapeString(f.Field), html.EscapeString(f.Package), html.EscapeString(f.Locator)))
		b.WriteString(fmt.Sprintf(`<span class="field-locator">%s</span>`, html.EscapeString(f.Locator)))
		b.WriteString(fmt.Sprintf(`<span class="field-value">%s</span>`, html.EscapeString(f.Value)))
		b.WriteString(fmt.Sprintf(`<p class="field-excerpt">%s</p>`, html.EscapeString(f.Excerpt)))
		b.WriteString(`</div>`)
	}
	if len(n.Devices) > 0 {
		b.WriteString(`<section class="device-lists">`)
		for pkg, items := range n.Devices {
			b.WriteString(fmt.Sprintf(`<div class="device-list" data-package="%s"><ul>`, html.EscapeString(pkg)))
			for _, it := range items {
				b.WriteString(fmt.Sprintf(`<li>%s</li>`, html.EscapeString(it)))
			}
			b.WriteString(`</ul></div>`)
		}
		b.WriteString(`</section>`)
	}
	if n.Complete {
		b.WriteString(fmt.Sprintf(`<div id="content-complete" %s="true"></div>`, MarkerContentOK))
	}
	// 不完整时 article/body 标签刻意不闭合，模拟半份下载内容
	if n.Complete {
		b.WriteString(`</article></body></html>`)
	}
	return b.String()
}
