import { useMemo, useState } from "react";
import { Popover } from "antd";
import { Check, ChevronDown, FolderTree } from "lucide-react";

import { type AigcProjectTreeNode } from "@/services/api/aigc";

type AigcProjectTreePickerProps = {
    value?: number;
    tree: AigcProjectTreeNode[];
    loading?: boolean;
    error?: string;
    disabled?: boolean;
    required?: boolean;
    allowClear?: boolean;
    placeholder?: string;
    buttonClassName?: string;
    className?: string;
    onChange: (value?: number) => void;
};

export function AigcProjectTreePicker({
    value,
    tree,
    loading = false,
    error = "",
    disabled = false,
    required = false,
    allowClear = false,
    placeholder = "选择业务项目",
    buttonClassName = "",
    className = "",
    onChange,
}: AigcProjectTreePickerProps) {
    const [open, setOpen] = useState(false);
    const selected = useMemo(() => findTreeNode(tree, value), [tree, value]);
    const label = loading ? "项目读取中..." : selected?.project.projectName || placeholder;
    const selectableCount = countSelectableProjects(tree);

    const handleChange = (next?: number) => {
        onChange(next);
        setOpen(false);
    };

    const menu = (
        <div className={`creation-project-menu aigc-project-tree-picker ${className}`.trim()}>
            <div className="creation-project-heading">
                <span>{required ? "业务项目" : "业务项目（可选）"}</span>
                <strong>{selectableCount ? `${selectableCount} 个` : "无可用项目"}</strong>
            </div>
            {loading ? (
                <div className="creation-project-empty">正在读取项目树</div>
            ) : error ? (
                <div className="creation-project-empty">{error}</div>
            ) : tree.length ? (
                <div className="aigc-project-tree-scroll creation-project-options" role="listbox" aria-label="选择业务项目">
                    {!required && allowClear ? (
                        <button type="button" role="option" aria-selected={!value} className={!value ? "is-selected" : ""} onClick={() => handleChange(undefined)}>
                            <span>不选择业务项目</span>
                            <small>可留空</small>
                            {!value ? <Check /> : null}
                        </button>
                    ) : null}
                    {tree.map((node) => (
                        <TreeNodeItem
                            key={node.project.projectId}
                            node={node}
                            selectedId={value}
                            depth={0}
                            onSelect={handleChange}
                        />
                    ))}
                </div>
            ) : (
                <div className="creation-project-empty">暂无可用项目</div>
            )}
        </div>
    );

    return (
        <Popover
            open={open}
            onOpenChange={(next) => {
                if (disabled) return;
                setOpen(next);
            }}
            trigger="click"
            placement="bottomLeft"
            arrow={false}
            classNames={{ root: "creation-control-popover", container: "creation-control-popover-surface", content: "creation-control-popover-content" }}
            content={menu}
        >
            <button
                type="button"
                disabled={disabled}
                className={buttonClassName}
                aria-label={label}
            >
                <FolderTree />
                <span>{label}</span>
                <ChevronDown className={open ? "is-open" : ""} />
            </button>
        </Popover>
    );
}

function TreeNodeItem({
    node,
    selectedId,
    depth,
    onSelect,
}: {
    node: AigcProjectTreeNode;
    selectedId?: number;
    depth: number;
    onSelect: (value?: number) => void;
}) {
    const project = node.project;
    const selected = project.projectId === selectedId;
    const disabled = project.status !== "启用";
    const label = depth === 0 ? "一级项目" : "二级项目";
    return (
        <div className="aigc-project-tree-branch">
            <button
                type="button"
                role="option"
                aria-selected={selected}
                className={`creation-project-option aigc-project-tree-node ${selected ? "is-selected" : ""} ${disabled ? "is-disabled" : ""}`}
                disabled={disabled}
                style={{ paddingInlineStart: `${8 + depth * 14}px` }}
                onClick={() => onSelect(project.projectId)}
            >
                <span className="aigc-project-tree-node-main">
                    <span className="aigc-project-tree-node-title">{project.projectName}</span>
                    <small>{project.projectType || label}</small>
                </span>
                <span className="aigc-project-tree-node-meta">{label}</span>
                {selected ? <Check /> : null}
            </button>
            {node.children?.length ? (
                <div className="aigc-project-tree-children">
                    {node.children.map((child) => (
                        <TreeNodeItem key={child.project.projectId} node={child} selectedId={selectedId} depth={depth + 1} onSelect={onSelect} />
                    ))}
                </div>
            ) : null}
        </div>
    );
}

function findTreeNode(tree: AigcProjectTreeNode[], value?: number): AigcProjectTreeNode | undefined {
    if (!value) return undefined;
    for (const node of tree) {
        if (node.project.projectId === value) return node;
        const child = node.children ? findTreeNode(node.children, value) : undefined;
        if (child) return child;
    }
    return undefined;
}

function countSelectableProjects(tree: AigcProjectTreeNode[]) {
    let count = 0;
    const visit = (nodes: AigcProjectTreeNode[]) => {
        for (const node of nodes) {
            if (node.project.status === "启用") count += 1;
            if (node.children?.length) visit(node.children);
        }
    };
    visit(tree);
    return count;
}
