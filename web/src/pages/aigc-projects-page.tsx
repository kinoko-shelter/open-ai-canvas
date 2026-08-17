import { App, Button, Drawer, Form, Input, Select, Table, Tree } from "antd";
import type { DataNode } from "antd/es/tree";
import { FolderTree, Pencil, Plus, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { ListToolbar, PaginationBar, TableSurface } from "@/components/layout/workspace-page";
import { AdminRowActions, AdminTableEmpty, AdminTableSkeleton } from "@/pages/admin/components/admin-ui";
import { AdminPageFrame } from "@/pages/admin/components/admin-shell";
import { aigcProjectLevelLabel, createAigcProject, listAigcDepartments, listAigcProjects, updateAigcProject, type AigcDepartment, type AigcProject, type AigcProjectInput } from "@/services/api/aigc";

const ALL_PROJECTS_KEY = "__all_projects";
type ProjectTreeKey = typeof ALL_PROJECTS_KEY | number;

export default function AigcProjectsPage() {
    const { message } = App.useApp();
    const [projects, setProjects] = useState<AigcProject[]>([]);
    const [treeProjects, setTreeProjects] = useState<AigcProject[]>([]);
    const [departments, setDepartments] = useState<AigcDepartment[]>([]);
    const [keyword, setKeyword] = useState("");
    const [status, setStatus] = useState("all");
    const [selectedProjectId, setSelectedProjectId] = useState<ProjectTreeKey>(ALL_PROJECTS_KEY);
    const [page, setPage] = useState(1);
    const [total, setTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [editing, setEditing] = useState<AigcProject | null>(null);
    const [drawerLevel, setDrawerLevel] = useState<1 | 2>(1);
    const [drawerOpen, setDrawerOpen] = useState(false);
    const [expandedKeys, setExpandedKeys] = useState<number[]>([]);
    const [form] = Form.useForm<AigcProjectInput>();
    const formParentId = Form.useWatch("parentId", form);

    const reload = async (nextPage = page) => {
        setLoading(true);
        try {
            const [result, allProjects, departmentResult] = await Promise.all([
                listAigcProjects({ keyword: keyword || undefined, status: status === "all" ? undefined : status, level: "2", page: nextPage, limit: 20 }),
                listAllAigcProjects(),
                listAigcDepartments(),
            ]);
            if (selectedProjectId === ALL_PROJECTS_KEY) {
                setProjects(result.projects);
                setTotal(result.total);
            } else {
                const scopedProjects = projectScope(allProjects, selectedProjectId, keyword, status);
                setProjects(scopedProjects.slice((nextPage - 1) * 20, nextPage * 20));
                setTotal(scopedProjects.length);
            }
            setTreeProjects(allProjects);
            setDepartments(departmentResult.departments);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取项目失败");
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => { void reload(); }, [page, selectedProjectId, status]);
    const parents = useMemo(() => treeProjects.filter((item) => item.level === 1), [treeProjects]);
    const parentNames = useMemo(() => new Map(parents.map((item) => [item.projectId, item.projectName])), [parents]);
    const departmentNames = useMemo(() => new Map(departments.map((item) => [item.deptId, item.name])), [departments]);
    const selectedProject = useMemo(() => treeProjects.find((item) => item.projectId === selectedProjectId), [selectedProjectId, treeProjects]);
    const selectedTitle = selectedProjectId === ALL_PROJECTS_KEY ? "全部项目" : selectedProject?.projectName || "当前项目";

    useEffect(() => {
        setExpandedKeys(parents.map((item) => item.projectId));
    }, [parents]);

    const openDrawer = (project?: AigcProject, level: 1 | 2 = project?.level || 1) => {
        setEditing(project || null);
        setDrawerLevel(level);
        form.resetFields();
        const parentId = level === 2 && selectedProject?.level === 1 ? selectedProject.projectId : undefined;
        form.setFieldsValue(project ? project : { status: "启用", level, parentId, deptId: undefined, projectName: "", projectType: "默认", projectNameOuter: "", projectDesc: "", remark: "" });
        setDrawerOpen(true);
    };

    const treeData = buildProjectTree(treeProjects, openDrawer);

    const save = async () => {
        const values = await form.validateFields();
        setSaving(true);
        try {
            const level = drawerLevel;
            const input: AigcProjectInput = { ...values, level, projectName: values.projectName.trim(), projectType: "默认", projectNameOuter: "", projectDesc: values.projectDesc?.trim() || "", remark: values.remark?.trim() || "", parentId: level === 2 ? values.parentId : undefined, deptId: level === 2 ? values.deptId : undefined };
            await (editing ? updateAigcProject(editing.projectId, input) : createAigcProject(input));
            setDrawerOpen(false);
            await reload();
            message.success(editing ? "项目已更新" : "项目已创建");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存项目失败");
        } finally {
            setSaving(false);
        }
    };

    const renderAddActions = () => <div className="flex flex-wrap items-center gap-2"><Button icon={<Plus className="size-4" />} onClick={() => openDrawer(undefined, 1)}>添加一级项目</Button><Button type="primary" icon={<Plus className="size-4" />} disabled={!parents.length} onClick={() => openDrawer(undefined, 2)}>添加二级项目</Button></div>;

    return <AdminPageFrame title="项目管理" description="维护运营项目及一级、二级项目层级" actions={renderAddActions()}>
        <ListToolbar active={Boolean(keyword || status !== "all" || selectedProjectId !== ALL_PROJECTS_KEY)} onReset={() => { setKeyword(""); setStatus("all"); setSelectedProjectId(ALL_PROJECTS_KEY); setPage(1); }}>
            <Input className="app-list-search" allowClear prefix={<Search className="size-4 text-foreground/40" />} value={keyword} placeholder="搜索项目名称" onChange={(event) => setKeyword(event.target.value)} onPressEnter={() => { setPage(1); void reload(1); }} />
            <Select className="w-28" value={status} onChange={(value) => { setStatus(value); setPage(1); }} options={[{ label: "全部状态", value: "all" }, { label: "启用", value: "启用" }, { label: "禁用", value: "禁用" }]} />
            <Button onClick={() => { setPage(1); void reload(1); }}>筛选</Button>
        </ListToolbar>
        <div className="aigc-projects-split">
            <aside className="aigc-projects-tree-panel">
                <button type="button" className={selectedProjectId === ALL_PROJECTS_KEY ? "aigc-projects-tree-root is-active" : "aigc-projects-tree-root"} onClick={() => { setSelectedProjectId(ALL_PROJECTS_KEY); setPage(1); }}>
                    <FolderTree className="size-4" />
                    <span>全部项目</span>
                    <em>{treeProjects.filter((item) => item.level === 2).length}</em>
                </button>
                <Tree blockNode autoExpandParent expandedKeys={expandedKeys} selectedKeys={[selectedProjectId]} treeData={treeData} onExpand={(keys) => setExpandedKeys(keys as number[])} onSelect={(keys) => { setSelectedProjectId((keys[0] || ALL_PROJECTS_KEY) as ProjectTreeKey); setPage(1); }} />
            </aside>
            <TableSurface className="aigc-projects-table-panel">
                <div className="aigc-projects-table-heading"><strong>{selectedTitle}</strong><span>{selectedProjectId === ALL_PROJECTS_KEY ? "全部二级项目" : "当前项目及子项目"}</span></div>
                {loading && projects.length === 0 ? <AdminTableSkeleton rows={8} columns={8} /> : <><Table className="app-data-table" rowKey="projectId" loading={loading} pagination={false} scroll={{ x: 1040 }} columns={[
                    { title: "ID", dataIndex: "projectId", width: 90 },
                    { title: "项目名称", dataIndex: "projectName", render: (_, item) => <ProjectNameCell project={item} /> },
                    { title: "类型", dataIndex: "projectType", width: 130 },
                    { title: "级别", dataIndex: "level", width: 90, render: aigcProjectLevelLabel },
                    { title: "上级项目", dataIndex: "parentId", width: 150, render: (value) => value ? parentNames.get(value) || "--" : "--" },
                    { title: "团队", dataIndex: "deptId", width: 150, render: (value) => value ? <div><div>{departmentNames.get(value) || "未知团队"}</div><div className="text-xs text-foreground/45">ID: {value}</div></div> : "全局" },
                    { title: "状态", dataIndex: "status", width: 90 },
                    { title: "操作", width: 120, fixed: "right", align: "right", render: (_, item) => <AdminRowActions actions={[{ key: "edit", label: "编辑项目", icon: <Pencil className="size-3.5" />, onClick: () => openDrawer(item) }]} /> },
                ]} dataSource={projects} locale={{ emptyText: <AdminTableEmpty title="当前范围没有项目" description="选择左侧项目查看当前项目及其子项目。" action={renderAddActions()} /> }} /><PaginationBar current={page} pageSize={20} total={total} onChange={(next) => setPage(next)} /></>}
            </TableSurface>
        </div>
        <Drawer title={editing ? "编辑项目" : "添加项目"} open={drawerOpen} width="min(640px, 100vw)" onClose={() => setDrawerOpen(false)} destroyOnHidden extra={<Button type="primary" loading={saving} onClick={() => void save()}>保存</Button>}>
            <Form form={form} layout="vertical" requiredMark={false}>
                <Form.Item name="projectName" label="项目名称" rules={[{ required: true, whitespace: true, message: "请填写项目名称" }]}><Input /></Form.Item>
                {drawerLevel === 2 ? <Form.Item name="parentId" label="上级项目" rules={[{ required: true, message: "请选择一级项目" }]}><Select options={parents.filter((item) => item.projectId !== editing?.projectId).map((item) => ({ label: item.projectName, value: item.projectId }))} /></Form.Item> : null}
                {drawerLevel === 2 && formParentId ? <Form.Item name="deptId" label="团队" rules={[{ required: true, message: "请选择团队" }]}><Select options={departments.map((item) => ({ label: `${item.name} · ID: ${item.deptId}`, value: item.deptId, disabled: item.status === "禁用" }))} /></Form.Item> : null}
                <Form.Item name="status" label="状态" rules={[{ required: true }]}><Select options={[{ label: "启用", value: "启用" }, { label: "禁用", value: "禁用" }]} /></Form.Item>
                <Form.Item name="projectDesc" label="项目说明"><Input.TextArea rows={4} /></Form.Item>
                <Form.Item name="remark" label="备注"><Input.TextArea rows={3} /></Form.Item>
            </Form>
        </Drawer>
    </AdminPageFrame>;
}

async function listAllAigcProjects() {
    const result: AigcProject[] = [];
    let page = 1;
    let total = 0;
    do {
        const batch = await listAigcProjects({ page, limit: 100 });
        result.push(...batch.projects);
        total = batch.total;
        page += 1;
    } while (result.length < total);
    return result;
}

function buildProjectTree(projects: AigcProject[], onEdit?: (project: AigcProject) => void): DataNode[] {
    const childrenByParent = new Map<number | "", AigcProject[]>();
    projects.forEach((project) => {
        childrenByParent.set(project.parentId || "", [...(childrenByParent.get(project.parentId || "") || []), project]);
    });
    const build = (parentId: number | ""): DataNode[] => (childrenByParent.get(parentId) || [])
        .sort((left, right) => left.projectId - right.projectId)
        .map((project) => ({
            key: project.projectId,
            title: <ProjectTreeTitle project={project} childCount={(childrenByParent.get(project.projectId) || []).length} onEdit={onEdit} />,
            children: project.level === 1 ? build(project.projectId) : undefined,
        }));
    return build("");
}

function projectScope(projects: AigcProject[], selectedProjectId: number, keyword: string, status: string) {
    const childrenByParent = new Map<number, AigcProject[]>();
    projects.forEach((project) => {
        if (!project.parentId) return;
        childrenByParent.set(project.parentId, [...(childrenByParent.get(project.parentId) || []), project]);
    });
    const result: AigcProject[] = [];
    const walk = (projectId: number) => {
        const project = projects.find((item) => item.projectId === projectId);
        if (!project) return;
        result.push(project);
        (childrenByParent.get(project.projectId) || []).forEach((child) => walk(child.projectId));
    };
    walk(selectedProjectId);
    const normalizedKeyword = keyword.trim().toLowerCase();
    return result
        .filter((project, index) => {
            if (index === 0) return true;
            if (status !== "all" && project.status !== status) return false;
            if (!normalizedKeyword) return true;
            return project.projectName.toLowerCase().includes(normalizedKeyword) || (project.projectNameOuter || "").toLowerCase().includes(normalizedKeyword);
        })
        .sort((left, right) => left.projectId - right.projectId);
}

function ProjectTreeTitle({ project, childCount, onEdit }: { project: AigcProject; childCount: number; onEdit?: (project: AigcProject) => void }) {
    return (
        <span className="aigc-projects-tree-title">
            <span>{project.projectName}</span>
            {project.level === 1 ? <em>{childCount}</em> : null}
            {project.level === 1 && onEdit ? <button type="button" className="grid size-5 place-items-center rounded text-foreground/45 hover:bg-surface-hover hover:text-foreground" title="编辑一级项目" aria-label={`编辑一级项目 ${project.projectName}`} onClick={(event) => { event.stopPropagation(); onEdit(project); }}><Pencil className="size-3" /></button> : null}
        </span>
    );
}

function ProjectNameCell({ project }: { project: AigcProject }) {
    return <div className="font-medium">{project.projectName}</div>;
}
