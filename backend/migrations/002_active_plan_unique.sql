-- 迁移 002：一块地同时只允许存在一条未完成种植计划。
--
-- 部分唯一索引（partial unique index）：
--   * 仅对 status <> 'completed' 的计划生效，已完成的历史计划不受限制；
--   * 计划完成（地块转 harvested）并重新认养后，允许再创建新计划；
--   * 并发提交由数据库兜底：第二个插入被拒绝，整个事务回滚，不留半条计划。
--
-- 幂等：IF NOT EXISTS，可重复执行。PostgreSQL 9.5+ 支持 CREATE INDEX IF NOT EXISTS。
-- 应用侧 GORM AutoMigrate 也会按 model tag 创建同名索引，二者一致、互不冲突。

CREATE UNIQUE INDEX IF NOT EXISTS idx_one_active_plan_per_plot
    ON planting_plans(plot_id)
    WHERE status <> 'completed';

-- plot_id 普通查询索引（部分唯一索引不能完全覆盖等值查询场景，保持查询计划稳定）。
CREATE INDEX IF NOT EXISTS idx_plans_plot ON planting_plans(plot_id);
