package sampledata

// 样本公告的结构化描述：由 render.go 统一渲染为带固定标记的 HTML，
// 后端 parser 依赖这些标记提取采购编号、更正指向、字段与原文位置。

type FieldDef struct {
	Field   string // deadline / contact / budget / device_list / address ...
	Package string // 空串=项目级字段
	Locator string // 原文位置（章节条款）
	Value   string
	Excerpt string // 原文片段
}

type PackageDef struct {
	No    string
	Title string
}

type Notice struct {
	Ref         string
	DocType     string // notice / correction / cancellation / cancel_correction
	ProjectNo   string // 显式采购编号；空串表示缺失（触发人工关联候选）
	Title       string
	PublishTime string
	TargetRef   string // 更正明确引用的原公告/更正站内编号
	CorrectionNo string
	ScopeAll    bool
	ScopePkgs   []string
	Packages    []PackageDef
	Fields      []FieldDef
	Devices      map[string][]string // 标包设备清单
	Complete    bool                 // false => 半份内容（缺少 CONTENT_COMPLETE）
}

type ListEntry struct {
	Ref         string
	Title       string
	PublishTime string
}

const (
	PageEndMarker   = "PW-SAMPLE-END-V1"
	CursorEndMarker = "CW-SAMPLE-END-V1"
	ReviseEndMarker = "RW-SAMPLE-END-V1"
)

// 站点 A：页码分页。A-001 在第 1、2 页重复出现（重复发现）；A-005 详情 404（失效链接）。
func PageSitePages() [][]ListEntry {
	return [][]ListEntry{
		{
			{Ref: "A-001", Title: "市立医院影像设备采购项目招标公告", PublishTime: "2026-11-01T09:00:00Z"},
			{Ref: "A-002", Title: "市立医院影像设备采购项目标包设备清单补充公告", PublishTime: "2026-11-02T10:00:00Z"},
			{Ref: "A-003", Title: "市立医院影像设备维保服务项目招标公告", PublishTime: "2026-11-03T10:00:00Z"},
		},
		{
			{Ref: "A-001", Title: "市立医院影像设备采购项目招标公告", PublishTime: "2026-11-01T09:00:00Z"}, // 重复发现
			{Ref: "A-004", Title: "某单位办公设备采购公告（编号暂缺）", PublishTime: "2026-11-04T10:00:00Z"},
			{Ref: "A-005", Title: "档案数字化服务采购公告（链接已失效）", PublishTime: "2026-11-05T10:00:00Z"},
		},
		// 第 3 页为空且带结束标志 => 到尾
	}
}

func PageSiteNotices() map[string]Notice {
	return map[string]Notice{
		"A-001": {
			Ref: "A-001", DocType: "notice", ProjectNo: "PRJ-A-100",
			Title: "市立医院影像设备采购项目招标公告", PublishTime: "2026-11-01T09:00:00Z",
			Packages: []PackageDef{{"PKG-01", "CT扫描仪包"}, {"PKG-02", "MRI设备包"}},
			Fields: []FieldDef{
				{Field: "deadline", Package: "PKG-01", Locator: "第一章 3.2条", Value: "2026-12-01 17:00", Excerpt: "PKG-01（CT扫描仪包）报名截止时间为2026年12月1日17:00，逾期不予受理。"},
				{Field: "deadline", Package: "PKG-02", Locator: "第一章 3.3条", Value: "2026-12-02 17:00", Excerpt: "PKG-02（MRI设备包）报名截止时间为2026年12月2日17:00，逾期不予受理。"},
				{Field: "contact", Package: "", Locator: "第二章 1.1条", Value: "李工 010-88880001", Excerpt: "项目联系人：李工，联系电话010-88880001。"},
				{Field: "budget", Package: "", Locator: "第三章 2.1条", Value: "人民币 800 万元", Excerpt: "本项目预算金额人民币800万元，超过预算的投标将被拒绝。"},
			},
			Devices:  map[string][]string{},
			Complete: true,
		},
		"A-002": {
			Ref: "A-002", DocType: "notice", ProjectNo: "PRJ-A-100",
			Title: "市立医院影像设备采购项目标包设备清单补充公告", PublishTime: "2026-11-02T10:00:00Z",
			TargetRef: "A-001",
			Packages:  []PackageDef{{"PKG-01", "CT扫描仪包"}},
			Fields: []FieldDef{
				{Field: "device_list", Package: "PKG-01", Locator: "补充清单 第1节", Value: "64排CT 1台；高压注射器 1台；UPS电源 2套", Excerpt: "现补充PKG-01设备清单如下：64排CT 1台、高压注射器 1台、UPS电源 2套。"},
			},
			Devices: map[string][]string{
				"PKG-01": {"64排CT 1台", "高压注射器 1台", "UPS电源 2套"},
			},
			Complete: true,
		},
		"A-003": {
			Ref: "A-003", DocType: "notice", ProjectNo: "PRJ-A-101", // 标题与 A-001 相近但编号不同 => 不同项目
			Title:  "市立医院影像设备维保服务项目招标公告", PublishTime: "2026-11-03T10:00:00Z",
			Packages: []PackageDef{{"PKG-01", "维保服务包"}},
			Fields: []FieldDef{
				{Field: "deadline", Package: "PKG-01", Locator: "第一章 2.1条", Value: "2026-12-05 17:00", Excerpt: "维保服务包报名截止时间为2026年12月5日17:00。"},
				{Field: "contact", Package: "", Locator: "第二章 1.1条", Value: "王工 010-88880002", Excerpt: "联系人：王工，电话010-88880002。"},
			},
			Complete: true,
		},
		"A-004": {
			Ref: "A-004", DocType: "notice", ProjectNo: "", // 缺少采购编号 => 关联候选交人工确认
			Title:  "某单位办公设备采购公告（编号暂缺）", PublishTime: "2026-11-04T10:00:00Z",
			Packages: []PackageDef{{"PKG-01", "办公设备包"}},
			Fields: []FieldDef{
				{Field: "deadline", Package: "PKG-01", Locator: "第一章 4.1条", Value: "2026-12-08 12:00", Excerpt: "办公设备包报名截止时间为2026年12月8日12:00。"},
			},
			Complete: true,
		},
		// A-005 详情始终 404
	}
}

// 站点 B：游标分页。含部分标包更正、矛盾通知、无引用对象通知、撤标、取消更正。
func CursorSitePages() (seq [][]ListEntry, cursors []string) {
	seq = [][]ListEntry{
		{
			{Ref: "B-001", Title: "产业园配电工程采购项目招标公告", PublishTime: "2026-11-01T09:00:00Z"},
			{Ref: "B-002", Title: "产业园配电工程采购项目更正公告（第1号）", PublishTime: "2026-11-03T09:00:00Z"},
			{Ref: "B-003", Title: "产业园配电工程采购项目更正公告（第2号）", PublishTime: "2026-11-04T09:00:00Z"},
		},
		{
			{Ref: "B-004", Title: "产业园配电工程采购项目更正公告（第3号）", PublishTime: "2026-11-05T09:00:00Z"},
			{Ref: "B-005", Title: "产业园配电工程采购项目更正公告（第4号）", PublishTime: "2026-11-06T09:00:00Z"},
			{Ref: "B-006", Title: "产业园配电工程采购项目情况说明（未注明引用对象）", PublishTime: "2026-11-07T09:00:00Z"},
		},
		{
			{Ref: "B-007", Title: "园区绿化养护项目撤标公告", PublishTime: "2026-11-08T09:00:00Z"},
			{Ref: "B-008", Title: "园区绿化养护项目招标公告", PublishTime: "2026-11-02T09:00:00Z"},
			{Ref: "B-009", Title: "关于取消《更正公告（第1号）》的通知", PublishTime: "2026-11-09T09:00:00Z"},
		},
		// 最后一个游标返回空列表 + 结束标志
	}
	cursors = []string{"", "c1", "c2", "c3"}
	return
}

func CursorSiteNotices() map[string]Notice {
	return map[string]Notice{
		"B-001": {
			Ref: "B-001", DocType: "notice", ProjectNo: "PRJ-B-200",
			Title: "产业园配电工程采购项目招标公告", PublishTime: "2026-11-01T09:00:00Z",
			Packages: []PackageDef{{"PKG-01", "高压柜包"}, {"PKG-02", "变压器包"}, {"PKG-03", "电缆包"}},
			Fields: []FieldDef{
				{Field: "deadline", Package: "PKG-01", Locator: "第一章 3.1条", Value: "2026-12-10 17:00", Excerpt: "高压柜包报名截止时间为2026年12月10日17:00。"},
				{Field: "deadline", Package: "PKG-02", Locator: "第一章 3.2条", Value: "2026-12-10 17:00", Excerpt: "变压器包报名截止时间为2026年12月10日17:00。"},
				{Field: "deadline", Package: "PKG-03", Locator: "第一章 3.3条", Value: "2026-12-11 17:00", Excerpt: "电缆包报名截止时间为2026年12月11日17:00。"},
				{Field: "budget", Package: "", Locator: "第三章 1.1条", Value: "人民币 1200 万元", Excerpt: "本项目预算1200万元。"},
				{Field: "contact", Package: "", Locator: "第四章 1.1条", Value: "赵工 021-66660001", Excerpt: "联系人赵工，电话021-66660001。"},
			},
			Complete: true,
		},
		"B-002": {
			Ref: "B-002", DocType: "correction", ProjectNo: "PRJ-B-200",
			Title: "产业园配电工程采购项目更正公告（第1号）", PublishTime: "2026-11-03T09:00:00Z",
			TargetRef: "B-001", CorrectionNo: "COR-B-200-01", ScopePkgs: []string{"PKG-02"},
			Fields: []FieldDef{
				{Field: "deadline", Package: "PKG-02", Locator: "更正事项 第1项", Value: "2026-12-15 17:00", Excerpt: "因技术参数调整，PKG-02变压器包报名截止时间更正为2026年12月15日17:00，其余标包不变。"},
			},
			Complete: true,
		},
		"B-003": {
			Ref: "B-003", DocType: "correction", ProjectNo: "PRJ-B-200",
			Title: "产业园配电工程采购项目更正公告（第2号）", PublishTime: "2026-11-04T09:00:00Z",
			TargetRef: "B-001", CorrectionNo: "COR-B-200-02", ScopeAll: true,
			Fields: []FieldDef{
				{Field: "contact", Package: "", Locator: "更正事项 第1项", Value: "赵工 021-66668888", Excerpt: "联系人电话统一更正为021-66668888，适用于本项目全部标包。"},
			},
			Complete: true,
		},
		"B-004": {
			Ref: "B-004", DocType: "correction", ProjectNo: "PRJ-B-200",
			Title: "产业园配电工程采购项目更正公告（第3号）", PublishTime: "2026-11-05T09:00:00Z",
			TargetRef: "B-001", CorrectionNo: "COR-B-200-03", ScopeAll: true,
			Fields: []FieldDef{
				{Field: "budget", Package: "", Locator: "更正事项 第1项", Value: "人民币 1350 万元", Excerpt: "因主材涨价，项目预算更正为1350万元。"},
			},
			Complete: true,
		},
		"B-005": {
			Ref: "B-005", DocType: "correction", ProjectNo: "PRJ-B-200",
			Title: "产业园配电工程采购项目更正公告（第4号）", PublishTime: "2026-11-06T09:00:00Z",
			TargetRef: "B-001", CorrectionNo: "COR-B-200-04", ScopeAll: true,
			Fields: []FieldDef{
				{Field: "budget", Package: "", Locator: "更正事项 第1项", Value: "人民币 1280 万元", Excerpt: "经复核，项目预算应为1280万元，特此更正。"},
			},
			Complete: true,
		},
		"B-006": {
			Ref: "B-006", DocType: "correction", ProjectNo: "PRJ-B-200",
			Title:  "产业园配电工程采购项目情况说明（未注明引用对象）", PublishTime: "2026-11-07T09:00:00Z",
			TargetRef: "", CorrectionNo: "COR-B-200-05", ScopeAll: true, // 发布时间晚但无明确引用对象
			Fields: []FieldDef{
				{Field: "deadline", Package: "PKG-01", Locator: "说明 第1段", Value: "2026-12-20 17:00", Excerpt: "有关事项说明如下，相关时间安排请知悉（未注明针对哪份公告、作用范围不明）。"},
			},
			Complete: true,
		},
		"B-008": {
			Ref: "B-008", DocType: "notice", ProjectNo: "PRJ-B-210",
			Title:  "园区绿化养护项目招标公告", PublishTime: "2026-11-02T09:00:00Z",
			Packages: []PackageDef{{"PKG-01", "绿化养护包"}},
			Fields: []FieldDef{
				{Field: "deadline", Package: "PKG-01", Locator: "第一章 2.2条", Value: "2026-12-06 17:00", Excerpt: "绿化养护包报名截止时间为2026年12月6日17:00。"},
			},
			Complete: true,
		},
		"B-007": {
			Ref: "B-007", DocType: "cancellation", ProjectNo: "PRJ-B-210",
			Title:  "园区绿化养护项目撤标公告", PublishTime: "2026-11-08T09:00:00Z",
			TargetRef: "B-008", CorrectionNo: "CAN-B-210-01", ScopeAll: true,
			Fields: []FieldDef{
				{Field: "__withdrawal__", Package: "", Locator: "撤标决定 第1段", Value: "撤标", Excerpt: "因采购计划调整，经研究决定撤销PRJ-B-210园区绿化养护项目本次招标，已报名单位另行通知。"},
			},
			Complete: true,
		},
		"B-009": {
			Ref: "B-009", DocType: "cancel_correction", ProjectNo: "PRJ-B-200",
			Title:  "关于取消《更正公告（第1号）》的通知", PublishTime: "2026-11-09T09:00:00Z",
			TargetRef: "B-002", CorrectionNo: "CC-B-200-01", ScopePkgs: []string{"PKG-02"},
			Fields: []FieldDef{
				// 取消通知自身载明恢复后的取值，不简单回退最早值
				{Field: "deadline", Package: "PKG-02", Locator: "取消通知 第1项", Value: "2026-12-10 17:00", Excerpt: "现取消COR-B-200-01号更正公告，PKG-02变压器包报名截止时间按本通知载明执行，即2026年12月10日17:00。"},
			},
			Complete: true,
		},
	}
}

// 站点 C：详情原址修订。C-001 同 URL 换文；C-002 初始为半份内容，可修复。
func ReviseSiteList() []ListEntry {
	return []ListEntry{
		{Ref: "C-001", Title: "数据中心服务器采购项目招标公告", PublishTime: "2026-11-01T09:00:00Z"},
		{Ref: "C-002", Title: "存储扩容采购项目招标公告（内容不完整）", PublishTime: "2026-11-02T09:00:00Z"},
		{Ref: "C-003", Title: "机房空调采购项目招标公告", PublishTime: "2026-11-03T09:00:00Z"},
	}
}

func ReviseSiteNotices() map[string]map[string]Notice {
	return map[string]map[string]Notice{
		"stage1": {
			"C-001": {
				Ref: "C-001", DocType: "notice", ProjectNo: "PRJ-C-300",
				Title: "数据中心服务器采购项目招标公告", PublishTime: "2026-11-01T09:00:00Z",
				Packages: []PackageDef{{"PKG-01", "服务器包"}},
				Fields: []FieldDef{
					{Field: "deadline", Package: "PKG-01", Locator: "第一章 3.1条", Value: "2026-12-18 17:00", Excerpt: "服务器包报名截止时间为2026年12月18日17:00。"},
					{Field: "contact", Package: "", Locator: "第二章 1.1条", Value: "孙工 0755-22220001", Excerpt: "联系人孙工，电话0755-22220001。"},
				},
				Complete: true,
			},
			"C-002": {
				Ref: "C-002", DocType: "notice", ProjectNo: "PRJ-C-301",
				Title:  "存储扩容采购项目招标公告（内容不完整）", PublishTime: "2026-11-02T09:00:00Z",
				Packages: []PackageDef{{"PKG-01", "存储包"}},
				Fields: []FieldDef{
					{Field: "deadline", Package: "PKG-01", Locator: "第一章 3.1条", Value: "2026-12-19 17:00", Excerpt: "存储包报名截止时间为2026年12月19日17:00。"},
					// 正文在 contact 章节中途截断
				},
				Complete: false,
			},
			"C-003": {
				Ref: "C-003", DocType: "notice", ProjectNo: "PRJ-C-302",
				Title:  "机房空调采购项目招标公告", PublishTime: "2026-11-03T09:00:00Z",
				Packages: []PackageDef{{"PKG-01", "空调包"}},
				Fields: []FieldDef{
					{Field: "deadline", Package: "PKG-01", Locator: "第一章 3.1条", Value: "2026-12-22 17:00", Excerpt: "空调包报名截止时间为2026年12月22日17:00。"},
				},
				Complete: true,
			},
		},
		"stage2": {
			"C-001": {
				Ref: "C-001", DocType: "notice", ProjectNo: "PRJ-C-300",
				Title: "数据中心服务器采购项目招标公告", PublishTime: "2026-11-01T09:00:00Z", // 原址换文：URL 不变
				Packages: []PackageDef{{"PKG-01", "服务器包"}},
				Fields: []FieldDef{
					{Field: "deadline", Package: "PKG-01", Locator: "第一章 3.1条", Value: "2026-12-25 17:00", Excerpt: "【修订】服务器包报名截止时间调整为2026年12月25日17:00。"},
					{Field: "contact", Package: "", Locator: "第二章 1.1条", Value: "孙工 0755-22220001", Excerpt: "联系人孙工，电话0755-22220001。"},
				},
				Complete: true,
			},
			"C-002": {
				Ref: "C-002", DocType: "notice", ProjectNo: "PRJ-C-301",
				Title:  "存储扩容采购项目招标公告", PublishTime: "2026-11-02T09:00:00Z",
				Packages: []PackageDef{{"PKG-01", "存储包"}},
				Fields: []FieldDef{
					{Field: "deadline", Package: "PKG-01", Locator: "第一章 3.1条", Value: "2026-12-19 17:00", Excerpt: "存储包报名截止时间为2026年12月19日17:00。"},
					{Field: "contact", Package: "", Locator: "第二章 1.1条", Value: "周工 0755-22220002", Excerpt: "联系人周工，电话0755-22220002。"},
					{Field: "budget", Package: "", Locator: "第三章 1.1条", Value: "人民币 300 万元", Excerpt: "项目预算300万元。"},
				},
				Complete: true, // 修复后完整
			},
			"C-003": {
				Ref: "C-003", DocType: "notice", ProjectNo: "PRJ-C-302",
				Title:  "机房空调采购项目招标公告", PublishTime: "2026-11-03T09:00:00Z",
				Packages: []PackageDef{{"PKG-01", "空调包"}},
				Fields: []FieldDef{
					{Field: "deadline", Package: "PKG-01", Locator: "第一章 3.1条", Value: "2026-12-22 17:00", Excerpt: "空调包报名截止时间为2026年12月22日17:00。"},
				},
				Complete: true,
			},
		},
	}
}
