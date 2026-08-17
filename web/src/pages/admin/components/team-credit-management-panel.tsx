import { useEffect, useState } from "react";
import { App, Button, Drawer, Form, Input, InputNumber, Modal, Select, Table, Tag } from "antd";
import type { ColumnsType } from "antd/es/table";
import { Coins, HandCoins, RefreshCw, ScrollText, UsersRound } from "lucide-react";

import { TableSurface } from "@/components/layout/workspace-page";
import { formatCredits } from "@/constant/credits";
import { getAdminTeamCreditManagement, transferAdminTeamCredits, type AdminTeamCreditLead, type AdminTeamCreditMember } from "@/services/api/wallet";

import { AdminUserDetailDrawer } from "./admin-user-detail-drawer";

type TransferFormValues = {
    recipientUserId: string;
    amount: number;
    note: string;
};

const maxSafeCreditInput = Math.floor(Number.MAX_SAFE_INTEGER / 1_000_000);

function createTransferRequestKey() {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") return crypto.randomUUID();
    return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export default function TeamCreditManagementPanel() {
    const { message } = App.useApp();
    const [transferForm] = Form.useForm<TransferFormValues>();
    const [leads, setLeads] = useState<AdminTeamCreditLead[]>([]);
    const [loading, setLoading] = useState(true);
    const [transferLead, setTransferLead] = useState<AdminTeamCreditLead | null>(null);
    const [membersLead, setMembersLead] = useState<AdminTeamCreditLead | null>(null);
    const [detailUserId, setDetailUserId] = useState<string | null>(null);
    const [transferring, setTransferring] = useState(false);
    const [transferRequestKey, setTransferRequestKey] = useState("");

    const reload = async () => {
        setLoading(true);
        try {
            const result = await getAdminTeamCreditManagement();
            setLeads(Array.isArray(result.leads) ? result.leads.map((lead) => ({ ...lead, members: Array.isArray(lead.members) ? lead.members : [] })) : []);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "读取团队积分失败");
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        void reload();
    }, []);

    const openTransfer = (lead: AdminTeamCreditLead) => {
        transferForm.resetFields();
        setTransferRequestKey(createTransferRequestKey());
        setTransferLead(lead);
    };

    const submitTransfer = async () => {
        if (!transferLead) return;
        try {
            const values = await transferForm.validateFields();
            const amountMicrocredits = Math.round(Number(values.amount) * 1_000_000);
            if (!Number.isSafeInteger(amountMicrocredits) || amountMicrocredits <= 0) {
                message.error("充值积分格式无效");
                return;
            }
            if (amountMicrocredits > transferLead.availableMicrocredits) {
                message.error("该团队主管可用积分不足，无法完成本次充值");
                return;
            }
            const idempotencyKey = transferRequestKey || createTransferRequestKey();
            setTransferRequestKey(idempotencyKey);
            setTransferring(true);
            const result = await transferAdminTeamCredits({
                senderUserId: transferLead.id,
                recipientUserId: values.recipientUserId,
                amountMicrocredits,
                note: values.note.trim(),
                idempotencyKey,
            });
            setTransferLead(null);
            transferForm.resetFields();
            await reload();
            message.success(result.replayed ? "已确认本次团队积分划拨到账" : "已从团队主管账户划拨积分");
        } catch (error) {
            if (error instanceof Error) message.error(error.message);
        } finally {
            setTransferring(false);
        }
    };

    const columns: ColumnsType<AdminTeamCreditLead> = [
        {
            title: "团队",
            width: 190,
            render: (_, lead) => (
                <div className="min-w-0">
                    <div className="truncate font-medium">{lead.departmentName}</div>
                    <div className="mt-1 flex items-center gap-2 text-xs text-foreground/50">
                        <span>ID: {lead.deptId}</span>
                        <Tag variant="filled" color={lead.departmentStatus === "启用" ? "success" : "default"}>{lead.departmentStatus}</Tag>
                    </div>
                </div>
            ),
        },
        {
            title: "团队主管",
            width: 190,
            render: (_, lead) => (
                <div className="min-w-0">
                    <div className="truncate font-medium">{lead.displayName || lead.username}</div>
                    <div className="mt-1 truncate text-xs text-foreground/50">@{lead.username}</div>
                </div>
            ),
        },
        {
            title: "主管可用积分",
            dataIndex: "availableMicrocredits",
            width: 140,
            align: "right",
            render: (value) => <span className="font-medium tabular-nums">{formatCredits(value, 6)}</span>,
        },
        {
            title: "冻结积分",
            dataIndex: "reservedMicrocredits",
            width: 120,
            align: "right",
            render: (value) => <span className="tabular-nums text-foreground/65">{formatCredits(value, 6)}</span>,
        },
        {
            title: "团队成员",
            width: 140,
            render: (_, lead) => <Button type="link" icon={<UsersRound className="size-3.5" />} onClick={() => setMembersLead(lead)}>{lead.members?.length ?? 0} 人</Button>,
        },
        {
            title: "操作",
            width: 220,
            fixed: "right",
            render: (_, lead) => (
                <div className="flex items-center gap-1">
                    <Button type="text" size="small" icon={<ScrollText className="size-3.5" />} onClick={() => setDetailUserId(lead.id)}>查看流水</Button>
                    <Button type="primary" size="small" icon={<HandCoins className="size-3.5" />} disabled={!lead.canTransfer} onClick={() => openTransfer(lead)}>代主管充值</Button>
                </div>
            ),
        },
    ];

    const memberColumns: ColumnsType<AdminTeamCreditMember> = [
        {
            title: "团队成员",
            render: (_, member) => (
                <div className="min-w-0">
                    <div className="truncate font-medium">{member.displayName || member.username}</div>
                    <div className="mt-1 truncate text-xs text-foreground/50">@{member.username}</div>
                </div>
            ),
        },
        { title: "可用积分", dataIndex: "availableMicrocredits", width: 140, align: "right", render: (value) => <span className="font-medium tabular-nums">{formatCredits(value, 6)}</span> },
        { title: "冻结积分", dataIndex: "reservedMicrocredits", width: 120, align: "right", render: (value) => <span className="tabular-nums text-foreground/65">{formatCredits(value, 6)}</span> },
        { title: "流水", width: 104, render: (_, member) => <Button type="text" size="small" icon={<ScrollText className="size-3.5" />} onClick={() => setDetailUserId(member.id)}>查看</Button> },
    ];

    return (
        <>
            <section className="flex flex-col gap-3 border-b border-border/75 py-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex min-w-0 items-start gap-3">
                    <span className="grid size-9 shrink-0 place-items-center rounded-md border border-border bg-muted/40"><Coins className="size-4" /></span>
                    <div className="min-w-0">
                        <h2 className="text-base font-semibold">主管账户划拨</h2>
                        <p className="mt-1 text-xs leading-5 text-foreground/55">管理员代操作时只会扣除所选团队主管的可用积分，并写入双方流水与管理员审计。</p>
                    </div>
                </div>
                <Button icon={<RefreshCw className="size-4" />} loading={loading} onClick={() => void reload()}>刷新</Button>
            </section>

            <TableSurface className="mt-5">
                <Table
                    className="app-data-table"
                    rowKey="id"
                    size="middle"
                    loading={loading}
                    columns={columns}
                    dataSource={leads}
                    pagination={false}
                    tableLayout="fixed"
                    scroll={{ x: 1120 }}
                    locale={{ emptyText: "没有可管理的启用团队主管" }}
                />
            </TableSurface>

            <Drawer
                title={membersLead ? `${membersLead.departmentName} · 团队成员` : "团队成员"}
                open={Boolean(membersLead)}
                onClose={() => setMembersLead(null)}
                width={760}
                destroyOnHidden
            >
                <div className="mb-5 flex flex-wrap items-center justify-between gap-3 border-b border-border/70 pb-4">
                    <div>
                        <div className="text-sm font-medium">主管：{membersLead?.displayName || membersLead?.username}</div>
                        <div className="mt-1 text-xs text-foreground/55">可用积分 {formatCredits(membersLead?.availableMicrocredits || 0, 6)} · 冻结 {formatCredits(membersLead?.reservedMicrocredits || 0, 6)}</div>
                    </div>
                    {membersLead?.canTransfer ? <Button type="primary" icon={<HandCoins className="size-4" />} onClick={() => { if (membersLead) { setMembersLead(null); openTransfer(membersLead); } }}>代主管充值</Button> : null}
                </div>
                <Table className="app-data-table" rowKey="id" size="middle" columns={memberColumns} dataSource={membersLead?.members || []} pagination={false} tableLayout="fixed" />
            </Drawer>

            <Modal
                title={transferLead ? `代 ${transferLead.displayName || transferLead.username} 向成员充值` : "代团队主管充值"}
                open={Boolean(transferLead)}
                onCancel={() => setTransferLead(null)}
                onOk={() => void submitTransfer()}
                confirmLoading={transferring}
                okText="确认划拨"
                cancelText="取消"
                okButtonProps={{ disabled: !transferLead?.canTransfer }}
                destroyOnHidden
            >
                <div className="mb-5 rounded-md border border-border bg-surface px-3 py-2 text-sm text-foreground/70">
                    扣款账户：{transferLead?.displayName || transferLead?.username}，当前可用 {formatCredits(transferLead?.availableMicrocredits || 0, 6)} 积分。
                </div>
                <Form
                    form={transferForm}
                    layout="vertical"
                    requiredMark={false}
                    onValuesChange={() => {
                        if (!transferring) setTransferRequestKey(createTransferRequestKey());
                    }}
                >
                    <Form.Item name="recipientUserId" label="团队成员" rules={[{ required: true, message: "请选择团队成员" }]}>
                        <Select
                            showSearch
                            optionFilterProp="label"
                            placeholder="选择该主管团队内的成员"
                            options={(transferLead?.members || []).map((member) => ({ value: member.id, label: `${member.displayName || member.username} · @${member.username}` }))}
                        />
                    </Form.Item>
                    <Form.Item name="amount" label="划拨积分" rules={[{ required: true, message: "请输入划拨积分" }, { type: "number", min: 0.000001, max: maxSafeCreditInput, message: "请输入有效的划拨积分" }]}>
                        <InputNumber className="w-full" min={0.000001} max={maxSafeCreditInput} precision={6} step={1} addonAfter="积分" placeholder="输入本次划拨数量" />
                    </Form.Item>
                    <Form.Item name="note" label="划拨说明" rules={[{ required: true, whitespace: true, message: "请填写划拨说明" }, { max: 240, message: "划拨说明最多 240 个字符" }]}>
                        <Input.TextArea autoSize={{ minRows: 3, maxRows: 5 }} maxLength={240} placeholder="例如：本周项目创作额度" />
                    </Form.Item>
                </Form>
            </Modal>

            <AdminUserDetailDrawer userId={detailUserId} onClose={() => setDetailUserId(null)} />
        </>
    );
}
