-- 采购公告采集与更正追溯平台 初始化 schema
-- PostgreSQL 13+

CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin','buyer')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 采集来源（样本站点的三个站点各一行）
CREATE TABLE sources (
    id               BIGSERIAL PRIMARY KEY,
    code             TEXT NOT NULL UNIQUE,
    name             TEXT NOT NULL,
    base_url         TEXT NOT NULL,
    list_mode        TEXT NOT NULL CHECK (list_mode IN ('page','cursor')),
    list_path        TEXT NOT NULL,             -- 例如 /page-site/list 或 /cursor-site/list
    detail_path_tpl  TEXT NOT NULL,             -- 例如 /page-site/notice/{ref}
    interval_ms      INTEGER NOT NULL DEFAULT 1000,   -- 同站两次请求最小间隔
    max_concurrency  INTEGER NOT NULL DEFAULT 2,
    max_retries      INTEGER NOT NULL DEFAULT 3,
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,   -- 暂停来源：FALSE 仅停止新领取
    end_marker       TEXT NOT NULL,              -- 样本站点声明的结束标志（空列表尾标记）
    cursor_state     JSONB NOT NULL DEFAULT '{}'::jsonb, -- 列表队列断点（页码/游标）
    list_status      TEXT NOT NULL DEFAULT 'idle'
        CHECK (list_status IN ('idle','running','done','error')),
    last_error       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 列表采集作业（整站串行租约；epoch 防止中断恢复后旧执行者回写）
CREATE TABLE list_jobs (
    id           BIGSERIAL PRIMARY KEY,
    source_id    BIGINT NOT NULL REFERENCES sources(id),
    kind         TEXT NOT NULL DEFAULT 'full' CHECK (kind IN ('full','manual')),
    status       TEXT NOT NULL CHECK (status IN ('pending','running','succeeded','failed')),
    epoch        INTEGER NOT NULL DEFAULT 1,
    worker_id    TEXT NOT NULL DEFAULT '',
    lease_until  TIMESTAMPTZ,
    last_offset  JSONB NOT NULL DEFAULT '{}'::jsonb, -- 本作业已保存到的断点
    error        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ
);
CREATE INDEX idx_list_jobs_dispatch ON list_jobs(status, lease_until);
-- 同一来源同时只允许一个未完成作业
CREATE UNIQUE INDEX uq_list_jobs_open_per_source
    ON list_jobs(source_id) WHERE status IN ('pending','running');

-- 列表页发现记录：发现即落库，详情失败不影响发现
CREATE TABLE discovery_items (
    id           BIGSERIAL PRIMARY KEY,
    source_id    BIGINT NOT NULL REFERENCES sources(id),
    source_ref   TEXT NOT NULL,            -- 站内公告编号/路径
    url          TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    detail_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (detail_status IN ('pending','done','failed','inaccessible')),
    UNIQUE (source_id, source_ref)
);
CREATE INDEX idx_discovery_detail_status ON discovery_items(source_id, detail_status);

-- 详情下载作业（与列表队列分开排队，按来源受并发与间隔限制）
CREATE TABLE detail_jobs (
    id            BIGSERIAL PRIMARY KEY,
    source_id     BIGINT NOT NULL REFERENCES sources(id),
    source_ref    TEXT NOT NULL,
    url           TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','succeeded','failed','inaccessible')),
    attempts      INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 3,
    worker_id     TEXT NOT NULL DEFAULT '',
    lease_until   TIMESTAMPTZ,
    last_error    TEXT NOT NULL DEFAULT '',
    manual        BOOLEAN NOT NULL DEFAULT FALSE, -- 手动补抓：同样受站点限额约束
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_id, source_ref)
);
CREATE INDEX idx_detail_jobs_dispatch ON detail_jobs(status, lease_until);

-- 公告文档（同一 URL 多次正文各成一版）
CREATE TABLE documents (
    id              BIGSERIAL PRIMARY KEY,
    source_id       BIGINT NOT NULL REFERENCES sources(id),
    source_ref      TEXT NOT NULL,
    url             TEXT NOT NULL,
    doc_type        TEXT NOT NULL DEFAULT 'notice'
        CHECK (doc_type IN ('notice','correction','cancel_correction','cancellation')),
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_id, source_ref)
);

-- 文档正文版本（每次正文与指纹都保留；解析失败也保留原件与错误）
CREATE TABLE document_versions (
    id             BIGSERIAL PRIMARY KEY,
    document_id    BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    fetch_run_id   BIGINT NOT NULL DEFAULT 0,   -- 由哪次详情作业产生
    fingerprint    TEXT NOT NULL,               -- sha256(规范化正文)
    raw_html       TEXT NOT NULL DEFAULT '',
    parsed_ok      BOOLEAN NOT NULL DEFAULT FALSE,
    parse_error    TEXT NOT NULL DEFAULT '',    -- 半份内容/解析失败原因
    truncated      BOOLEAN NOT NULL DEFAULT FALSE,
    parsed_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    fetched_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id, fingerprint)
);
CREATE INDEX idx_doc_versions_doc ON document_versions(document_id, fetched_at);

-- 采购项目（项目级）
CREATE TABLE projects (
    id              BIGSERIAL PRIMARY KEY,
    project_no      TEXT,                      -- 显式采购编号；候选阶段可能为空
    title           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'candidate'
        CHECK (status IN ('candidate','active','withdrawn','closed')),
    status_reason   TEXT NOT NULL DEFAULT '',  -- 状态依据说明（如撤标公告编号）
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_checked_at TIMESTAMPTZ,               -- 最近一次正文核查时间
    check_note      TEXT NOT NULL DEFAULT ''   -- 长期未查提示，不声称仍有效
);
CREATE UNIQUE INDEX uq_projects_no ON projects(project_no) WHERE project_no IS NOT NULL;

-- 标包（项目下分包；项目与标包分别计数）
CREATE TABLE packages (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    package_no  TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, package_no)
);

-- 待人工确认的关联候选（缺依据：编号缺失/冲突/标题相近）
CREATE TABLE link_candidates (
    id           BIGSERIAL PRIMARY KEY,
    document_id  BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    project_id   BIGINT REFERENCES projects(id) ON DELETE CASCADE, -- 系统猜测，可为空
    project_no_hint TEXT NOT NULL DEFAULT '',
    reason       TEXT NOT NULL,                 -- 缺依据原因
    status       TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','confirmed','rejected','recheck')),
    handled_by   BIGINT REFERENCES users(id),
    handled_at   TIMESTAMPTZ,
    note         TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_link_candidates_pending ON link_candidates(status, created_at);
CREATE UNIQUE INDEX uq_link_candidates_open
    ON link_candidates(document_id, COALESCE(project_id, 0))
    WHERE status = 'pending';

-- 更正通知主表
CREATE TABLE corrections (
    id                 BIGSERIAL PRIMARY KEY,
    document_id        BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    project_id         BIGINT REFERENCES projects(id),
    target_document_id BIGINT REFERENCES documents(id), -- 明确引用的原公告
    correction_no      TEXT NOT NULL DEFAULT '',
    scope_all_packages BOOLEAN NOT NULL DEFAULT FALSE,
    scope_package_nos  JSONB NOT NULL DEFAULT '[]'::jsonb,
    status             TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','contradictory','confirmed','returned','superseded')),
    contradictory_note TEXT NOT NULL DEFAULT '',
    handled_by         BIGINT REFERENCES users(id),
    handled_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id)
);
CREATE INDEX idx_corrections_status ON corrections(status, created_at);

-- 更正涉及的字段（含原文位置、原文片段、适用标包、逐字段处置）
CREATE TABLE correction_field_changes (
    id             BIGSERIAL PRIMARY KEY,
    correction_id  BIGINT NOT NULL REFERENCES corrections(id) ON DELETE CASCADE,
    field_name     TEXT NOT NULL,             -- deadline / contact / budget / device_list ...
    package_no     TEXT NOT NULL DEFAULT '',  -- 空串表示项目级字段
    old_value      TEXT NOT NULL DEFAULT '',  -- 处理时的当前采用值快照
    new_value      TEXT NOT NULL DEFAULT '',  -- 新提取值
    source_locator TEXT NOT NULL DEFAULT '',  -- 原文位置（章节/段落序号）
    source_excerpt TEXT NOT NULL DEFAULT '',  -- 原文片段
    decision       TEXT NOT NULL DEFAULT 'pending'
        CHECK (decision IN ('pending','confirmed','returned')),
    decided_by     BIGINT REFERENCES users(id),
    decided_at     TIMESTAMPTZ,
    UNIQUE (correction_id, field_name, package_no)
);

-- 采用结论（当前生效值按 项目+标包+字段 唯一；每次确认形成可追溯修订）
CREATE TABLE adopted_revisions (
    id              BIGSERIAL PRIMARY KEY,
    project_id      BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    package_no      TEXT NOT NULL DEFAULT '',       -- 空串=项目级
    field_name      TEXT NOT NULL,
    field_value     TEXT NOT NULL,
    source_version_id BIGINT NOT NULL REFERENCES document_versions(id),
    source_document_id BIGINT NOT NULL REFERENCES documents(id),
    correction_id   BIGINT REFERENCES corrections(id), -- 经更正确认则记录
    change_kind     TEXT NOT NULL DEFAULT 'initial'
        CHECK (change_kind IN ('initial','correction','cancel_correction')),
    adopted_by      BIGINT REFERENCES users(id), -- 系统初始采用时为空
    adopted_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    superseded_at   TIMESTAMPTZ,
    is_current      BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE UNIQUE INDEX uq_adopted_current
    ON adopted_revisions(project_id, package_no, field_name) WHERE is_current;
CREATE INDEX idx_adopted_project ON adopted_revisions(project_id, package_no);

-- 定期复查队列（近期项目定期复查正文；长期未查展示最后核查时间）
CREATE TABLE review_queue (
    id             BIGSERIAL PRIMARY KEY,
    project_id     BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    document_id    BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    due_at         TIMESTAMPTZ NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','running','done','inaccessible')),
    worker_id      TEXT NOT NULL DEFAULT '',
    lease_until    TIMESTAMPTZ,
    attempts       INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ
);
CREATE INDEX idx_review_dispatch ON review_queue(status, due_at);
CREATE UNIQUE INDEX uq_review_open ON review_queue(project_id, document_id)
    WHERE status IN ('pending','running');

CREATE TABLE review_runs (
    id             BIGSERIAL PRIMARY KEY,
    project_id     BIGINT NOT NULL REFERENCES projects(id),
    document_id    BIGINT NOT NULL REFERENCES documents(id),
    version_id     BIGINT REFERENCES document_versions(id),
    result         TEXT NOT NULL CHECK (result IN ('unchanged','changed','inaccessible','parse_failed')),
    fingerprint    TEXT NOT NULL DEFAULT '',
    note           TEXT NOT NULL DEFAULT '',
    checked_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_review_runs_project ON review_runs(project_id, checked_at DESC);

-- 导出记录（导出前重新核验所用公告是否被撤回；保留所用修订）
CREATE TABLE export_runs (
    id           BIGSERIAL PRIMARY KEY,
    project_id   BIGINT NOT NULL REFERENCES projects(id),
    requested_by BIGINT REFERENCES users(id),
    verify_note  TEXT NOT NULL DEFAULT '',
    payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
