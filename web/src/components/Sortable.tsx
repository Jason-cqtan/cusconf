import type { ReactNode } from "react";
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, arrayMove, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical } from "lucide-react";

/** Vertical drag-to-reorder list. `ids` must be unique and stable per item. */
export function SortableList<T>({ items, ids, onReorder, render }: {
  items: T[];
  ids: string[];
  onReorder: (items: T[]) => void;
  render: (item: T, index: number, handle: ReactNode) => ReactNode;
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const onDragEnd = (e: DragEndEvent) => {
    if (!e.over || e.active.id === e.over.id) return;
    const from = ids.indexOf(String(e.active.id));
    const to = ids.indexOf(String(e.over.id));
    if (from >= 0 && to >= 0) onReorder(arrayMove(items, from, to));
  };
  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
      <SortableContext items={ids} strategy={verticalListSortingStrategy}>
        {items.map((it, i) => (
          <SortableRow key={ids[i]} id={ids[i]}>{(handle) => render(it, i, handle)}</SortableRow>
        ))}
      </SortableContext>
    </DndContext>
  );
}

function SortableRow({ id, children }: { id: string; children: (handle: ReactNode) => ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });
  const handle = (
    <span {...attributes} {...listeners} className="cursor-grab touch-none text-zinc-400 hover:text-zinc-600" aria-label="拖动排序">
      <GripVertical size={16} />
    </span>
  );
  return (
    <div ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition, zIndex: isDragging ? 10 : undefined, position: "relative" }}>
      {children(handle)}
    </div>
  );
}
