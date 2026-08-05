/**
 * 表格排序 hook
 *
 * 为只读表格提供点击表头排序能力（纯前端，本地排序）。
 * 返回当前排序列/方向、切换函数，以及渲染表头用的辅助类型。
 */
import { useCallback, useMemo, useState } from 'react';

export type SortDirection = 'asc' | 'desc';

export interface SortState {
  key: string | null;
  direction: SortDirection;
}

/**
 * useTableSort — 管理排序列与方向。
 * accessors 字典的键即排序列标识（不要求是 T 的直接属性，
 * 以便对嵌套字段如 r.experiment.name 排序）。
 */
export function useTableSort<T>(
  rows: T[],
  accessors: Record<string, (row: T) => string | number>,
) {
  const [sort, setSort] = useState<SortState>({ key: null, direction: 'asc' });

  const toggleSort = useCallback((key: string) => {
    setSort((prev) => {
      if (prev.key === key) {
        return { key, direction: prev.direction === 'asc' ? 'desc' : 'asc' };
      }
      return { key, direction: 'asc' };
    });
  }, []);

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
