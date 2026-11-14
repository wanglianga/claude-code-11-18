-- 文档处理备注：记录“解析阻断/缺编号候选”等处理结论，便于页面展示未核实原因
ALTER TABLE documents ADD COLUMN IF NOT EXISTS process_note TEXT NOT NULL DEFAULT '';

-- 更正关联的项目可能在人工确认候选后才补齐，补充可空关联的更新索引
CREATE INDEX IF NOT EXISTS idx_documents_project_lookup
    ON documents(source_id, source_ref);
