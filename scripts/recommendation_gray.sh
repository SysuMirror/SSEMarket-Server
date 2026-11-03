#!/usr/bin/env bash
# 推荐流 UID 灰度规则运维脚本（直接读写 ssemarket.recommendation_gray_rules）。
#
# 用法：
#   ./scripts/recommendation_gray.sh list
#   ./scripts/recommendation_gray.sh add <uidMin> <uidMax> [备注]
#   ./scripts/recommendation_gray.sh enable <id>
#   ./scripts/recommendation_gray.sh disable <id>
#   ./scripts/recommendation_gray.sh delete <id>
#
# 说明：
#   - 规则区间闭区间，UID 落在任一 enabled 规则内即命中灰度；
#   - 用户显式「关闭」优先于灰度，显式「开启」不受灰度限制；
#   - 服务端设置缓存 1 分钟，脚本直连 DB 后最迟 1 分钟生效；走管理员 API 则立即生效。
#
# 管理员 API（更推荐，实时生效）：
#   GET    /api/auth/admin/recommendation/gray
#   POST   /api/auth/admin/recommendation/gray   {"id":0,"uidMin":1,"uidMax":1000,"enabled":true,"note":""}
#   DELETE /api/auth/admin/recommendation/gray/:id
set -euo pipefail

DB_CONTAINER="${DB_CONTAINER:-sse_market_db}"
MYSQL_USER="${MYSQL_USER:-root}"
MYSQL_PASS="${MYSQL_PASS:-admin}"

sql() {
  docker exec -i "$DB_CONTAINER" mysql -u"$MYSQL_USER" -p"$MYSQL_PASS" -N -e "use ssemarket; $1"
}

usage() {
  sed -n '2,17p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

cmd="${1:-}"
case "$cmd" in
  list)
    sql "SELECT id, uid_min, uid_max, enabled, note, updated_at FROM recommendation_gray_rules ORDER BY uid_min, id;"
    ;;
  add)
    min="${2:-}"; max="${3:-}"; note="${4:-}"
    if ! [[ "$min" =~ ^[0-9]+$ && "$max" =~ ^[0-9]+$ ]] || [ "$min" -le 0 ] || [ "$max" -lt "$min" ]; then
      echo "UID 区间不合法：add <uidMin> <uidMax> [备注]" >&2
      exit 1
    fi
    note_sql="${note//\'/\'\'}"
    sql "INSERT INTO recommendation_gray_rules (uid_min, uid_max, enabled, note, created_at, updated_at) VALUES ($min, $max, 1, '$note_sql', NOW(), NOW()); SELECT LAST_INSERT_ID();"
    ;;
  enable|disable)
    id="${2:-}"
    [[ "$id" =~ ^[0-9]+$ ]] || { echo "需要规则 id" >&2; exit 1; }
    enabled=0; [ "$cmd" = "enable" ] && enabled=1
    sql "UPDATE recommendation_gray_rules SET enabled=$enabled, updated_at=NOW() WHERE id=$id; SELECT ROW_COUNT();"
    ;;
  delete)
    id="${2:-}"
    [[ "$id" =~ ^[0-9]+$ ]] || { echo "需要规则 id" >&2; exit 1; }
    sql "DELETE FROM recommendation_gray_rules WHERE id=$id; SELECT ROW_COUNT();"
    ;;
  *)
    usage
    ;;
esac