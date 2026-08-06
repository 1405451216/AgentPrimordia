/**
 * 表格排序 hook
 *
 * 为只读表格提供点击表头排序能力（纯前端，本地排序）。
 * 返回当前排序列/方向、切换函数，以及渲染表头用的辅助类型。
 */
import { useCallback, useEffect, useMemo, useState } from 'react';

export type SortDirection = 'asc' | 'desc';

export interface SortState {
  key: string | null;
  direction: SortDirection;
}

/** 从 URL ?sort=&dir= 读取初始排序（供 incident review 场景跨刷新保留） */
function initialFromURL(): SortState {
  if (typeof window === 'undefined') return { key: null, direction: 'asc' };
  const params = new URLSearchParams(window.location.search);
  const key = params.get('sort');
  const dir = params.get('dir') === 'desc' ? 'desc' : 'asc';
  return { key, direction: dir };
}

/**
 * useTableSort — 管理排序列与方向，并持久化到 URL（?sort=&dir=）。
 * accessors 字典的键即排序列标识（不要求是 T 的直接属性，
 * 以便对嵌套字段如 r.experiment.name 排序）。
 */
export function useTableSort<T>(
  rows: T[],
  accessors: Record<string, (row: T) => string | number>,
  /** 用于区分同一 URL 下多个表格的查询参数前缀（默认空） */
  paramPrefix = '',
) {
  const [sort, setSort] = useState<SortState>(initialFromURL);

  const toggleSort = useCallback((key: string) => {
    setSort((prev) => {
      if (prev.key === key) {
        return { key, direction: prev.direction === 'asc' ? 'desc' : 'asc' };
      }
      return { key, direction: 'asc' };
    });
  }, []);

  // 排序变化时写入 URL（不触发导航，仅 replace）
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const sortKey = `${paramPrefix}sort`;
    const dirKey = `${paramPrefix}dir`;
    if (sort.key) {
      params.set(sortKey, sort.key);
      params.set(dirKey, sort.direction);
    } else {
      params.delete(sortKey);
      params.delete(dirKey);
    }
    const qs = params.toString();
    window.history.replaceState(null, '', qs ? `?${qs}` : window.location.pathname);
  }, [sort, paramPrefix]);

  const sortedRows = useMemo(() => {
    if (sort.key === null) return rows;
    const accessor = accessors[sort.key];
    if (!accessor) return rows;
    const factor = sort.direction === 'asc' ? 1 : -1;
    return [...rows].sort((a, b) => {
      const av = accessor(a);
      const bv = accessor(b);
      if (typeof av === 'number' && typeof bv === 'number') {
        return (av - bv) * factor;
      }
      return String(av).localeCompare(String(bv), 'zh-Hans-CN') * factor;
    });
  }, [rows, sort, accessors]);

  return { sortedRows, sort, toggleSort };
}

/** 渲染可排序表头单元格 */
export function SortableTh({
  children,
  sortKey,
  sort,
  onToggle,
}: {
  children: React.ReactNode;
  sortKey: string;
  sort: SortState;
  onToggle: (key: string) => void;
}) {
  const active = sort.key === sortKey;
  const arrow = active ? (sort.direction === 'asc' ? ' ▲' : ' ▼') : '';
  return (
    <th className="sortable" onClick={() => onToggle(sortKey)} aria-sort={active ? (sort.direction === 'asc' ? 'ascending' : 'descending') : 'none'}>
      <button type="button" className="sort-btn">
        {children}
        <span className="sort-arrow" aria-hidden="true">{arrow}</span>
      </button>
    </th>
  );
}
