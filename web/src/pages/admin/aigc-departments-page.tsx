import { App, Button, Drawer, Form, Input, Select, Table, Tag } from "antd";
import { Pencil, Plus, Search, UsersRound } from "lucide-react";
import { useEffect, useState } from "react";

import { ListToolbar, TableSurface } from "@/components/layout/workspace-page";
import { createAigcDepartment, listAigcDepartments, updateAigcDepartment, type AigcDepartment } from "@/services/api/aigc";
import { AdminPageFrame } from "./components/admin-shell";
import { AdminRowActions, AdminTableEmpty, AdminTableSkeleton } from "./components/admin-ui";

type TeamFormValues = { name: string; status: "启用" | "禁用"; remark?: string };

export default function AigcDepartmentsPage() {
    const { message } = App.useApp();
    const [departments, setDepartments] = useState<AigcDepartment[]>([]);
    const [keyword, setKeyword] = useState("");
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [editing, setEditing] = useState<AigcDepartment | null>(null);
    const [drawerOpen, setDrawerOpen] = useState(false);
    const [form] = Form.useForm<TeamFormValues>();

    const reload = async () => {
        setLoading(true);
        try {
            const result = await listAigcDepartments({ keyword: keyword || undefined });
            setDepartments(result.departments);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取团队失败");
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        void reload();
    }, []);

    const openDrawer = (team?: AigcDepartment) => {
        setEditing(team || null);
        form.resetFields();
        form.setFieldsValue(team ? { name: team.name, status: team.status, remark: team.remark || "" } : { name: "", status: "启用", remark: "" });
        setDrawerOpen(true);
    };

    const save = async () => {
        const values = await form.validateFields();
        setSaving(true);
        try {
            const input = { name: values.name.trim(), status: values.status, remark: values.remark?.trim() || "" };
            await (editing ? updateAigcDepartment(editing.deptId, input) : createAigcDepartment(input));
            setDrawerOpen(false);
            await reload();
            message.success(editing ? "团队已更新" : "团队已创建");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存团队失败");
        } finally {
            setSaving(false);
        }
    };

    return (
        <AdminPageFrame title="团队管理" description="维护项目归属团队" actions={<Button type="primary" icon={<Plus className="size-4" />} onClick={() => openDrawer()}>添加团队</Button>}>
            <ListToolbar active={Boolean(keyword)} onReset={() => { setKeyword(""); void reload(); }}>
                <Input className="app-list-search" allowClear prefix={<Search className="size-4 text-foreground/40" />} value={keyword} placeholder="搜索团队名称" onChange={(event) => setKeyword(event.target.value)} onPressEnter={() => void reload()} />
                <Button onClick={() => void reload()}>筛选</Button>
            </ListToolbar>
            <TableSurface className="border-y border-border">
                {loading && departments.length === 0 ? <AdminTableSkeleton rows={6} columns={5} /> : (
                    <Table
                        className="app-data-table"
                        rowKey="deptId"
                        columns={[
                            {
                                title: "团队名称",
                                dataIndex: "name",
                                render: (_, team) => (
                                    <span className="inline-flex min-w-0 items-center gap-2">
                                        <UsersRound className="size-4 shrink-0 text-foreground/45" />
                                        <span className="min-w-0">
                                            <span className="block truncate">{team.name}</span>
                                            <span className="block text-xs text-foreground/40">ID: {team.deptId}</span>
                                        </span>
                                    </span>
                                ),
                            },
                            { title: "状态", dataIndex: "status", width: 90, render: (value) => <Tag color={value === "启用" ? "success" : "default"}>{value}</Tag> },
                            { title: "备注", dataIndex: "remark", render: (value) => value || <span className="text-foreground/40">未填写</span> },
                            { title: "更新时间", dataIndex: "updatedAt", width: 180, render: formatTime },
                            { title: "操作", width: 120, align: "right", render: (_, team) => <AdminRowActions actions={[{ key: "edit", label: "编辑团队", icon: <Pencil className="size-3.5" />, onClick: () => openDrawer(team) }]} /> },
                        ]}
                        dataSource={departments}
                        pagination={false}
                        locale={{ emptyText: <AdminTableEmpty filtered={Boolean(keyword)} title="没有团队" description="添加团队后，可在用户和项目中选择归属团队。" /> }}
                    />
                )}
            </TableSurface>
            <Drawer title={editing ? "编辑团队" : "添加团队"} open={drawerOpen} width="min(520px, 100vw)" onClose={() => setDrawerOpen(false)} destroyOnHidden extra={<Button type="primary" loading={saving} onClick={() => void save()}>保存</Button>}>
                <Form form={form} layout="vertical" requiredMark={false}>
                    <Form.Item name="name" label="团队名称" rules={[{ required: true, whitespace: true, message: "请填写团队名称" }]}><Input placeholder="例如：短剧运营组" /></Form.Item>
                    <Form.Item name="status" label="状态" rules={[{ required: true }]}><Select options={[{ label: "启用", value: "启用" }, { label: "禁用", value: "禁用" }]} /></Form.Item>
                    <Form.Item name="remark" label="备注"><Input.TextArea rows={4} placeholder="团队说明" /></Form.Item>
                </Form>
            </Drawer>
        </AdminPageFrame>
    );
}

function formatTime(value?: string) {
    return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "--";
}
