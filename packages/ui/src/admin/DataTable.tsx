'use client';

import { useEffect, useState, type ReactNode } from 'react';

export interface Column<T> {
  key: string;
  header: string;
  render?: (item: T) => ReactNode;
  sortable?: boolean;
  /** Searched by default; set to `false` to exclude a column from search. */
  searchable?: boolean;
  /** Value used for search/sort when the cell is rendered custom. Defaults to `item[key]`. */
  getValue?: (item: T) => unknown;
}

interface DataTableProps<T> {
  columns: Column<T>[];
  data: T[];
  loading?: boolean;
  onRowClick?: (item: T) => void;
  actions?: (item: T) => ReactNode;
  emptyMessage?: string;
  pageSize?: number;
  searchable?: boolean;
  /** Stable row key. Defaults to `item.id` when present, else the index. */
  keyExtractor?: (item: T, index: number) => string;
}

function cellText<T>(col: Column<T>, item: T): string {
  const raw = col.getValue ? col.getValue(item) : (item as Record<string, unknown>)[col.key];
  if (raw === null || raw === undefined) return '';
  if (typeof raw === 'string' || typeof raw === 'number' || typeof raw === 'boolean')
    return String(raw);
  return '';
}

export function DataTable<T extends Record<string, unknown>>({
  columns,
  data,
  loading,
  onRowClick,
  actions,
  emptyMessage = 'No data found',
  pageSize = 15,
  searchable = true,
  keyExtractor,
}: DataTableProps<T>) {
  const [search, setSearch] = useState('');
  const [sortKey, setSortKey] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');
  const [page, setPage] = useState(1);

  // Keep the page in range when the data set (or filter) shrinks.
  useEffect(() => {
    setPage((p) => Math.min(Math.max(p, 1), Math.max(1, Math.ceil(data.length / pageSize)) || 1));
  }, [data.length, pageSize]);

  let filtered = data;
  if (search && searchable) {
    const lower = search.toLowerCase();
    const searchableCols = columns.filter((col) => col.searchable !== false);
    filtered = data.filter((item) =>
      searchableCols.some((col) => cellText(col, item).toLowerCase().includes(lower)),
    );
  }

  if (sortKey) {
    const sortCol = columns.find((col) => col.key === sortKey);
    filtered = [...filtered].sort((a, b) => {
      const aVal = sortCol ? cellText(sortCol, a) : '';
      const bVal = sortCol ? cellText(sortCol, b) : '';
      const cmp = aVal.localeCompare(bVal);
      return sortDir === 'asc' ? cmp : -cmp;
    });
  }

  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize));
  const safePage = Math.min(Math.max(page, 1), pageCount);
  const paged = filtered.slice((safePage - 1) * pageSize, safePage * pageSize);

  const handleSort = (key: string) => {
    if (sortKey === key) {
      setSortDir(sortDir === 'asc' ? 'desc' : 'asc');
    } else {
      setSortKey(key);
      setSortDir('asc');
    }
  };

  const rowKey = (item: T, index: number): string => {
    if (keyExtractor) return keyExtractor(item, index);
    const id = (item as Record<string, unknown>)['id'];
    return typeof id === 'string' || typeof id === 'number' ? String(id) : `row-${index}`;
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="h-8 w-8 animate-spin rounded-full border-b-2 border-brand" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {searchable && (
        <div className="flex items-center gap-2">
          <input
            type="text"
            placeholder="Search..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
            className="ui-input w-64"
          />
        </div>
      )}
      <div className="overflow-x-auto rounded-lg border border-line">
        <table className="ui-table">
          <thead>
            <tr>
              {columns.map((col) => (
                <th
                  key={col.key}
                  className="ui-th"
                  onClick={() => col.sortable && handleSort(col.key)}
                >
                  <div className="flex items-center gap-1">
                    {col.header}
                    {sortKey === col.key && <span>{sortDir === 'asc' ? '↑' : '↓'}</span>}
                  </div>
                </th>
              ))}
              {actions && <th className="ui-th text-right">Actions</th>}
            </tr>
          </thead>
          <tbody>
            {paged.length === 0 ? (
              <tr>
                <td
                  colSpan={columns.length + (actions ? 1 : 0)}
                  className="ui-td text-center text-text-muted"
                >
                  {emptyMessage}
                </td>
              </tr>
            ) : (
              paged.map((item, i) => (
                <tr
                  key={rowKey(item, i)}
                  className={`ui-tr-interactive ${onRowClick ? 'cursor-pointer' : ''}`}
                  onClick={() => onRowClick?.(item)}
                >
                  {columns.map((col) => (
                    <td key={col.key} className="ui-td">
                      {col.render ? col.render(item) : cellText(col, item)}
                    </td>
                  ))}
                  {actions && (
                    <td className="ui-td text-right" onClick={(e) => e.stopPropagation()}>
                      {actions(item)}
                    </td>
                  )}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      {pageCount > 1 && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-text-muted">
            Page {safePage} of {pageCount}
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => setPage(Math.max(1, safePage - 1))}
              disabled={safePage <= 1}
              className="ui-button ui-button-secondary"
            >
              Previous
            </button>
            <button
              onClick={() => setPage(Math.min(pageCount, safePage + 1))}
              disabled={safePage >= pageCount}
              className="ui-button ui-button-secondary"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
