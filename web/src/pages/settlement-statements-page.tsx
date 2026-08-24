import { App, Button, DatePicker, Form, Input, Modal, Select } from "antd";
import type { ColumnsType } from "antd/es/table";
import dayjs from "dayjs";
import { BadgeCheck, ReceiptText, RefreshCw } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router";

import { PageHeader, PaginationBar, WorkspacePage } from "@/components/layout/workspace-page";
import { formatCredits } from "@/constant/credits";
import type { AigcDepartment, AigcProject } from "@/services/api/aigc";
import {
    confirmAdminSettlementStatements,
    getAdminSettlementReferences,
    listAdminSettlementStatementOrders,
    listAdminSettlementStatements,
    type BillingOrder,
    type SettlementStatement,
    type SettlementStatus,
} from "@/services/api/wallet";
import { useUserStore } from "@/stores/use-user-store";

import { AdminDataTable, AdminFilterChip, AdminRowActions, AdminStatusBadge, AdminTableEmpty } from "./admin/components/admin-ui";

type ConfirmTarget = { kind: "single"; statements: SettlementStatement[] } | { kind: "batch"; statements: SettlementStatement[] };
type ConfirmFormValues = { note: string };

const STATUS_OPTIONS = [
    { label: "全部状态", value: "all" },
    { label: "未结算", value: "unsettled" },
    { label: "已结算", value: "settled" },
];

export default function SettlementStatementsPage() {
    const { message } = App.useApp();
    const [searchParams, setSearchParams] = useSearchParams();
    const user = useUserStore((state) => state.user);
    const [departments, setDepartments] = useState<AigcDepartment[]>([]);
    const [projects, setProjects] = useState<AigcProject[]>([]);
    const [statements, setStatements] = useState<SettlementStatement[]>([]);
    const [orders, setOrders] = useState<BillingOrder[]>([]);
    const [summary, setSummary] = useState({ month: currentMonth(), totalCount: 0, settledCount: 0, unsettledCount: 0, totalAmountMicrocredits: 0, settledAmountMicrocredits: 0, unsettledAmountMicrocredits: 0 });
    const [total, setTotal] = useState(0);
    const [loading, setLoading] = useState(true);
    const [selectedIds, setSelectedIds] = useState<number[]>([]);
    const [confirming, setConfirming] = useState(false);
    const [confirmTarget, setConfirmTarget] = useState<ConfirmTarget | null>(null);
    const [form] = Form.useForm<ConfirmFormValues>();
    const requestSequence = useRef(0);

    const view = searchParams.get("view") === "orders" ? "orders" : "statements";
    const month = normalizeMonth(searchParams.get("month"));
    const status = normalizeStatus(searchParams.get("status"));
    const deptId = nonNegativeInt(searchParams.get("deptId"));
    const userId = searchParams.get("userId") || "";
    const projectId = positiveInt(searchParams.get("projectId"));
    const settlementType = searchParams.get("settlementType") || "";
    const page = positiveInt(searchParams.get("page")) || 1;
    const pageSize = normalizePageSize(searchParams.get("pageSize"));
    const canManage = user?.status === "active" && (user.role === "admin" || user.role === "operations_manager");
    const hasFilters = Boolean(status !== "all" || deptId !== undefined || projectId);
    const confirmableMonth = previousMonth();
    const selectedUnsettledStatements = statements.filter((item) => selectedIds.includes(item.id) && isConfirmableStatement(item, confirmableMonth));

    const projectOptions = useMemo(() => projects.map((project) => ({ label: project.projectName, value: project.projectId, disabled: project.status === "禁用" })), [projects]);
    const departmentOptions = useMemo(() => departments.map((team) => ({ label: team.name, value: team.deptId, disabled: team.status === "禁用" })), [departments]);

    const updateUrl = (patch: Record<string, string | number | undefined>, replace = false) => {
        const next = new URLSearchParams(searchParams);
        Object.entries(patch).forEach(([key, value]) => {
            const isDefault = value === undefined || value === "" || (key === "month" && value === currentMonth()) || (key === "status" && value === "all") || (key === "page" && value === 1) || (key === "pageSize" && value === 20);
            if (isDefault) next.delete(key);
            else next.set(key, String(value));
        });
        setSearchParams(next, { replace });
    };

    useEffect(() => {
        if (!canManage) return;
        void getAdminSettlementReferences()
            .then((result) => {
                setDepartments(result.departments);
                setProjects(result.projects);
            })
            .catch((error) => message.error(error instanceof Error ? error.message : "读取筛选项失败"));
    }, [canManage, message]);

    const reload = () => {
        if (!canManage) return;
        const sequence = ++requestSequence.current;
        setLoading(true);
        if (view === "orders") {
            if (deptId === undefined || !settlementType) {
                setOrders([]);
                setTotal(0);
                setLoading(false);
                return;
            }
            void listAdminSettlementStatementOrders({ month, deptId, userId: userId || undefined, projectId: projectId || undefined, settlementType, status, page, limit: pageSize })
                .then((result) => {
                    if (sequence !== requestSequence.current) return;
                    setOrders(result.orders);
                    setTotal(result.total);
                })
                .catch((error) => sequence === requestSequence.current && message.error(error instanceof Error ? error.message : "读取消费明细失败"))
                .finally(() => sequence === requestSequence.current && setLoading(false));
            return;
        }
        void listAdminSettlementStatements({ month, status, deptId, projectId: projectId || undefined, page, limit: pageSize })
            .then((result) => {
                if (sequence !== requestSequence.current) return;
                setStatements(result.statements);
                setSummary(result.summary);
                setTotal(result.total);
                setSelectedIds([]);
            })
            .catch((error) => sequence === requestSequence.current && message.error(error instanceof Error ? error.message : "读取结算单失败"))
            .finally(() => sequence === requestSequence.current && setLoading(false));
    };

    useEffect(reload, [canManage, view, month, status, deptId, userId, projectId, settlementType, page, pageSize]);

    const openConfirm = (target: ConfirmTarget) => {
        setConfirmTarget(target);
        form.setFieldsValue({ note: "运营确认结算单" });
    };

    const confirm = async () => {
        if (!confirmTarget) return;
        const values = await form.validateFields();
        setConfirming(true);
        try {
            const result = await confirmAdminSettlementStatements({ items: confirmTarget.statements, note: values.note.trim() });
            message.success(result.confirmedCount > 0 ? `已确认 ${result.confirmedCount} 张结算单` : "所选结算单已是已结算状态");
            setConfirmTarget(null);
            setSelectedIds([]);
            reload();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "确认结算单失败");
        } finally {
            setConfirming(false);
        }
    };

    const openOrders = (statement: SettlementStatement) => {
        updateUrl({ view: "orders", month: statement.month, deptId: statement.deptId, userId: statement.userId, projectId: statement.aigcProjectId, settlementType: statement.settlementType, status: "all", page: 1 });
    };

    if (!canManage) {
        return <WorkspacePage><div className="py-10 text-center text-sm text-foreground/55">当前账号无结算单权限。</div></WorkspacePage>;
    }

    return (
        <WorkspacePage>
            <PageHeader
                title={view === "orders" ? "消费明细" : "结算单"}
                description={view === "orders" ? "按结算单范围查看计费订单" : "按月份、团队、项目和结算类型汇总计费"}
                actions={view === "orders" ? <Button onClick={() => updateUrl({ view: "", userId: "", settlementType: "", page: 1 })}>返回结算单</Button> : <Button icon={<RefreshCw className="size-4" />} loading={loading} onClick={reload}>刷新</Button>}
            />
            {view === "orders" ? (
                <OrderTable orders={orders} loading={loading} total={total} page={page} pageSize={pageSize} onPageChange={(nextPage, nextSize) => updateUrl({ page: nextSize !== pageSize ? 1 : nextPage, pageSize: nextSize })} />
            ) : (
                <>
                    <SummaryBand summary={summary} />
                    <AdminDataTable
                        toolbar={<DatePicker picker="month" allowClear={false} value={dayjs(month)} onChange={(value) => updateUrl({ month: value?.format("YYYY-MM") || currentMonth(), page: 1 })} />}
                        toolbarFilters={<><Select className="w-32" value={status} options={STATUS_OPTIONS} onChange={(value) => updateUrl({ status: value, page: 1 })} /><Select allowClear showSearch className="w-40" placeholder="团队" value={deptId ?? undefined} options={departmentOptions} optionFilterProp="label" onChange={(value) => updateUrl({ deptId: value, page: 1 })} /><Select allowClear showSearch className="w-48" placeholder="项目" value={projectId || undefined} options={projectOptions} optionFilterProp="label" onChange={(value) => updateUrl({ projectId: value, page: 1 })} /><Button type="primary" icon={<BadgeCheck className="size-3.5" />} disabled={selectedUnsettledStatements.length === 0} onClick={() => openConfirm({ kind: "batch", statements: selectedUnsettledStatements })}>批量确认{selectedUnsettledStatements.length ? ` ${selectedUnsettledStatements.length}` : ""}</Button></>}
                        toolbarActiveFilters={<>{status !== "all" ? <AdminFilterChip label={`状态：${statusText(status)}`} onRemove={() => updateUrl({ status: "all", page: 1 })} /> : null}{deptId !== undefined ? <AdminFilterChip label={`团队：${departments.find((item) => item.deptId === deptId)?.name || deptId}`} onRemove={() => updateUrl({ deptId: undefined, page: 1 })} /> : null}{projectId ? <AdminFilterChip label={`项目：${projects.find((item) => item.projectId === projectId)?.projectName || projectId}`} onRemove={() => updateUrl({ projectId: undefined, page: 1 })} /> : null}</>}
                        toolbarActive={hasFilters}
                        toolbarDefaultFiltersOpen
                        onReset={() => updateUrl({ status: "all", deptId: undefined, projectId: undefined, page: 1 })}
                        table={{ className: "app-data-table", rowKey: "id", size: "small", loading, pagination: false, columns: statementColumns(openOrders, openConfirm, confirmableMonth), dataSource: statements, rowSelection: { selectedRowKeys: selectedIds, preserveSelectedRowKeys: false, onChange: (keys) => setSelectedIds(keys.map(Number).filter((key) => Number.isFinite(key))), getCheckboxProps: (record) => ({ disabled: !isConfirmableStatement(record, confirmableMonth) }) }, scroll: { x: 1190 } }}
                        empty={<AdminTableEmpty filtered={hasFilters} title="暂无结算单" description="当前筛选范围内没有可计费订单。" />}
                        footer={<PaginationBar alwaysShow current={page} pageSize={pageSize} total={total} onChange={(nextPage, nextSize) => updateUrl({ page: nextSize !== pageSize ? 1 : nextPage, pageSize: nextSize })} />}
                    />
                </>
            )}
            <Modal title={confirmTarget?.kind === "batch" ? "批量确认结算单" : "确认结算单"} open={Boolean(confirmTarget)} okText="确认" cancelText="取消" confirmLoading={confirming} onCancel={() => !confirming && setConfirmTarget(null)} onOk={() => void confirm()}>
                <Form form={form} layout="vertical" requiredMark={false}>
                    <Form.Item name="note" label="确认依据" rules={[{ required: true, whitespace: true, message: "请填写确认依据" }]}>
                        <Input.TextArea rows={3} maxLength={500} />
                    </Form.Item>
                </Form>
            </Modal>
        </WorkspacePage>
    );
}

function SummaryBand({ summary }: { summary: { month: string; unsettledCount: number; settledCount: number; unsettledAmountMicrocredits: number; settledAmountMicrocredits: number } }) {
    return (
        <section className="mt-4 rounded-lg border border-border bg-card p-4">
            <div className="flex items-center gap-2 text-sm font-medium text-foreground/70"><ReceiptText className="size-4" />{summary.month} 结算汇总</div>
            <div className="mt-3 grid gap-3 md:grid-cols-2">
                <div className="rounded-md border border-amber-500/25 bg-amber-500/10 p-4">
                    <div className="text-sm font-medium text-foreground/70">未结算</div>
                    <div className="mt-2 text-3xl font-semibold tabular-nums">{formatSettlementCredits(summary.unsettledAmountMicrocredits)}</div>
                    <div className="mt-2 text-xs text-foreground/50">{summary.unsettledCount} 单待确认</div>
                </div>
                <div className="rounded-md border border-emerald-500/25 bg-emerald-500/10 p-4">
                    <div className="text-sm font-medium text-foreground/70">已结算</div>
                    <div className="mt-2 text-3xl font-semibold tabular-nums">{formatSettlementCredits(summary.settledAmountMicrocredits)}</div>
                    <div className="mt-2 text-xs text-foreground/50">{summary.settledCount} 单已确认</div>
                </div>
            </div>
        </section>
    );
}

function statementColumns(openOrders: (statement: SettlementStatement) => void, openConfirm: (target: ConfirmTarget) => void, confirmableMonth: string): ColumnsType<SettlementStatement> {
    return [
        { title: "ID", dataIndex: "id", width: 110, render: (value) => <span className="tabular-nums" title={String(value)}>{value}</span> },
        { title: "结算月份", dataIndex: "month", width: 120 },
        { title: "团队", dataIndex: "departmentName", width: 170 },
        { title: "项目", dataIndex: "aigcProjectName", width: 190 },
        { title: "结算类型", dataIndex: "settlementType", width: 110, render: capabilityText },
        { title: "订单数", dataIndex: "orderCount", width: 90, align: "right" },
        { title: "结算金额", dataIndex: "amountMicrocredits", width: 130, align: "right", render: (value) => <span className="font-medium tabular-nums">{formatCredits(value)}</span> },
        { title: "状态", dataIndex: "settlementStatus", width: 100, render: (value) => <AdminStatusBadge label={value === "settled" ? "已结算" : "未结算"} tone={value === "settled" ? "success" : "warning"} /> },
        { title: "操作", width: 150, align: "right", render: (_, statement) => <AdminRowActions primary={{ label: "查看", onClick: () => openOrders(statement) }} actions={[{ key: "confirm", label: "确认", icon: <BadgeCheck className="size-3.5" />, disabled: !isConfirmableStatement(statement, confirmableMonth), onClick: () => openConfirm({ kind: "single", statements: [statement] }) }]} /> },
    ];
}

function OrderTable({ orders, loading, total, page, pageSize, onPageChange }: { orders: BillingOrder[]; loading: boolean; total: number; page: number; pageSize: number; onPageChange: (page: number, pageSize: number) => void }) {
    const columns: ColumnsType<BillingOrder> = [
        { title: "创建时间", dataIndex: "createdAt", width: 170, render: formatTime },
        { title: "模型 / 场景", width: 220, render: (_, order) => <div><div className="font-medium">{order.model}</div><div className="mt-0.5 text-xs text-foreground/50">{order.scene || order.capability}</div></div> },
        { title: "结算类型", dataIndex: "capability", width: 100, render: capabilityText },
        { title: "金额", width: 120, align: "right", render: (_, order) => <span className="font-medium tabular-nums">{formatCredits(order.status === "settled" ? order.actualAmountMicrocredits || order.amountMicrocredits : order.amountMicrocredits)}</span> },
        { title: "状态", dataIndex: "status", width: 110, render: (value) => <AdminStatusBadge label={billingStatusText(value)} tone={value === "settled" ? "success" : "warning"} /> },
        { title: "上游请求", dataIndex: "providerRequestId", width: 190, ellipsis: true, render: (value) => value || "未获取" },
    ];
    return <AdminDataTable table={{ className: "app-data-table", rowKey: "id", size: "small", loading, pagination: false, columns, dataSource: orders, scroll: { x: 910 } }} empty={<AdminTableEmpty title="暂无消费明细" />} footer={<PaginationBar alwaysShow current={page} pageSize={pageSize} total={total} onChange={onPageChange} />} />;
}

function currentMonth() { return dayjs().format("YYYY-MM"); }
function previousMonth() { return dayjs().subtract(1, "month").format("YYYY-MM"); }
function isConfirmableStatement(statement: SettlementStatement, confirmableMonth: string) { return statement.month === confirmableMonth && statement.settlementStatus === "unsettled"; }
function normalizeMonth(value: string | null) { return value && /^\d{4}-\d{2}$/.test(value) ? value : currentMonth(); }
function normalizeStatus(value: string | null): SettlementStatus { return value === "settled" || value === "unsettled" ? value : "all"; }
function normalizePageSize(value: string | null) { const parsed = positiveInt(value) || 20; return [20, 50, 100].includes(parsed) ? parsed : 20; }
function positiveInt(value: string | null) { if (!value) return undefined; const parsed = Number(value); return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined; }
function nonNegativeInt(value: string | null) { if (value === null || value === "") return undefined; const parsed = Number(value); return Number.isInteger(parsed) && parsed >= 0 ? parsed : undefined; }
function formatTime(value?: string) { return value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "--"; }
function formatSettlementCredits(value: number) { return (value / 1_000_000).toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 }); }
function statusText(value: SettlementStatus) { return value === "settled" ? "已结算" : value === "unsettled" ? "未结算" : "全部"; }
function capabilityText(value: string) { return ({ text: "文生图", image: "图生图", video: "视频", audio: "音频" } as Record<string, string>)[value] || value || "--"; }
function billingStatusText(value: BillingOrder["status"]) { return ({ reserved: "已冻结", running: "运行中", uncertain: "待核对", settled: "已结算", refunded: "已退款" } as Record<string, string>)[value] || value; }
