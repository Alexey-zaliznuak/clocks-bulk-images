import { useEffect, useMemo, useRef, useState } from "react";
import { collectPads, nodeState, toggleNode, type PadNode } from "../padsTree";

export default function PadsTree({
  trees,
  selected,
  onChange,
  loading,
  error,
}: {
  trees: PadNode[];
  selected: number[];
  onChange: (pads: number[]) => void;
  loading?: boolean;
  error?: string;
}) {
  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const [open, setOpen] = useState<Set<string>>(() => defaultOpen(trees));

  useEffect(() => {
    setOpen((current) => {
      const next = new Set(current);
      for (const id of defaultOpen(trees)) next.add(id);
      return next;
    });
  }, [trees]);

  if (loading) {
    return <p className="text-sm text-slate-500">Загружаем места размещения…</p>;
  }
  if (error) {
    return <p className="text-sm text-red-600">{error}</p>;
  }
  if (trees.length === 0) {
    return (
      <p className="text-sm text-slate-500">
        Дерево площадок недоступно. Пустой выбор = лента ВК при загрузке.
      </p>
    );
  }

  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50/70 px-2 py-2">
      {trees.map((node) => (
        <Branch
          key={node.id || node.name}
          node={node}
          depth={0}
          selected={selectedSet}
          open={open}
          onToggleOpen={(id) => {
            setOpen((current) => {
              const next = new Set(current);
              if (next.has(id)) next.delete(id);
              else next.add(id);
              return next;
            });
          }}
          onToggle={(nextNode) => onChange(toggleNode(nextNode, selected))}
        />
      ))}
    </div>
  );
}

function Branch({
  node,
  depth,
  selected,
  open,
  onToggleOpen,
  onToggle,
}: {
  node: PadNode;
  depth: number;
  selected: Set<number>;
  open: Set<string>;
  onToggleOpen: (id: string) => void;
  onToggle: (node: PadNode) => void;
}) {
  const children = node.children || [];
  const key = node.id || node.name;
  const expanded = open.has(key) || depth === 0;
  const state = nodeState(node, selected);
  const pads = collectPads(node);
  const boxRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (boxRef.current) boxRef.current.indeterminate = state === "some";
  }, [state]);

  return (
    <div>
      <div className="flex items-center gap-1 rounded-lg px-1 py-0.5 hover:bg-white" style={{ paddingLeft: 8 + depth * 16 }}>
        {children.length > 0 ? (
          <button
            type="button"
            aria-label="Toggle"
            onClick={() => onToggleOpen(key)}
            className="flex h-6 w-6 shrink-0 items-center justify-center text-slate-400"
          >
            <svg viewBox="0 0 16 12" width="14" height="12" className={expanded ? "" : "rotate-[-90deg]"}>
              <path d="M3 4l5 5 5-5" fill="none" stroke="currentColor" strokeWidth="1.6" />
            </svg>
          </button>
        ) : (
          <span className="inline-block w-6" />
        )}
        <label className={`flex min-w-0 flex-1 cursor-pointer items-center gap-2 ${pads.length === 0 ? "opacity-60" : ""}`}>
          <input
            ref={boxRef}
            type="checkbox"
            className="h-4 w-4"
            checked={state === "all"}
            disabled={pads.length === 0}
            onChange={() => onToggle(node)}
          />
          <span className="truncate text-sm text-slate-800">{node.name}</span>
        </label>
      </div>
      {expanded && children.map((child) => (
        <Branch
          key={child.id || child.name}
          node={child}
          depth={depth + 1}
          selected={selected}
          open={open}
          onToggleOpen={onToggleOpen}
          onToggle={onToggle}
        />
      ))}
    </div>
  );
}

function defaultOpen(trees: PadNode[]): Set<string> {
  const open = new Set<string>();
  for (const node of trees) {
    if (node.id) open.add(node.id);
    if (node.name) open.add(node.name);
    for (const child of node.children || []) {
      if (/вконтакт/i.test(child.name) || (child.id || "").includes("Вконтакт")) {
        open.add(child.id || child.name);
      }
    }
  }
  return open;
}
