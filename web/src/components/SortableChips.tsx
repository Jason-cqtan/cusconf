import type { ReactNode } from "react";
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, arrayMove, rectSortingStrategy, sortableKeyboardCoordinates, useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { X } from "lucide-react";
import { cx } from "./ui";

/**
 * Wrapping row of chips that can be dragged into a new order.
 * `items` must be unique strings; `tone` picks a chip style per item.
 */
export function SortableChips({ items, onReorder, onRemove, tone, children }: {
  items: string[];
  onReorder: (items: string[]) => void;
  onRemove?: (item: string) => void;
  tone?: (item: string) => "group" | "node" | "builtin";
  children?: ReactNode; // trailing content, e.g. an "add" control
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const onDragEnd = (e: DragEndEvent) => {
    if (!e.over || e.active.id === e.over.id) return;
    const from = items.indexOf(String(e.active.id));
    const to = items.indexOf(String(e.over.id));
    if (from >= 0 && to >= 0) onReorder(arrayMove(items, from, to));
  };
  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
      <SortableContext items={items} strategy={rectSortingStrategy}>
        <div className="flex flex-wrap items-center gap-1.5">
          {items.map((it) => <Chip key={it} id={it} tone={tone?.(it) ?? "node"} onRemove={onRemove} />)}
          {children}
        </div>
      </SortableContext>
    </DndContext>
  );
}

const tones = {
  group: "border-indigo-300 bg-indigo-50 text-indigo-700 dark:border-indigo-800 dark:bg-indigo-950 dark:text-indigo-300",
  builtin: "border-amber-300 bg-amber-50 text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-300",
  node: "border-zinc-300 bg-white text-zinc-700 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-300",
};

function Chip({ id, tone, onRemove }: { id: string; tone: keyof typeof tones; onRemove?: (id: string) => void }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });
  return (
    <span
      ref={setNodeRef}
      {...attributes}
      {...listeners}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      title="拖动调整顺序"
      className={cx(
        "inline-flex cursor-grab touch-none items-center gap-1 rounded-full border py-0.5 text-xs select-none active:cursor-grabbing",
        onRemove ? "pr-1 pl-2.5" : "px-2.5",
        tones[tone],
        isDragging && "relative z-10 opacity-70 shadow",
      )}
    >
      {id}
      {onRemove && (
        <button
          type="button"
          aria-label={`移除 ${id}`}
          className="ml-0.5 rounded-full p-0.5 opacity-60 hover:text-red-600 hover:opacity-100"
          onPointerDown={(e) => e.stopPropagation()}
          onClick={() => onRemove(id)}
        >
          <X size={12} />
        </button>
      )}
    </span>
  );
}
