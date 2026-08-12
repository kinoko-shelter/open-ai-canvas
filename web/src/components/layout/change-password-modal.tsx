import { App, Form, Input, Modal } from "antd";
import { useEffect, useState } from "react";

import { changePassword } from "@/services/api/auth";
import { useUserStore, type LocalUser } from "@/stores/use-user-store";

type ChangePasswordFormValues = {
    currentPassword: string;
    newPassword: string;
    confirmPassword: string;
};

export function ChangePasswordModal({ open, onClose }: { open: boolean; onClose: () => void }) {
    const { message } = App.useApp();
    const setUser = useUserStore((state) => state.setUser);
    const [form] = Form.useForm<ChangePasswordFormValues>();
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        if (open) form.resetFields();
    }, [form, open]);

    const submit = async () => {
        const values = await form.validateFields();
        setSaving(true);
        try {
            const result = await changePassword({ currentPassword: values.currentPassword, newPassword: values.newPassword });
            setUser(result.user as LocalUser);
            message.success("密码已修改");
            onClose();
        } catch (error) {
            message.error(error instanceof Error ? error.message : "修改密码失败");
        } finally {
            setSaving(false);
        }
    };

    return (
        <Modal title="修改密码" open={open} okText="保存新密码" cancelText="取消" confirmLoading={saving} onOk={() => void submit()} onCancel={onClose} destroyOnHidden>
            <Form form={form} layout="vertical" requiredMark={false}>
                <Form.Item name="currentPassword" label="当前密码" rules={[{ required: true, message: "请输入当前密码" }]}>
                    <Input.Password autoComplete="current-password" placeholder="请输入当前密码" />
                </Form.Item>
                <Form.Item name="newPassword" label="新密码" rules={[{ required: true, message: "请输入新密码" }, { min: 8, message: "密码至少 8 位" }]}>
                    <Input.Password autoComplete="new-password" placeholder="至少 8 位" />
                </Form.Item>
                <Form.Item
                    name="confirmPassword"
                    label="确认新密码"
                    dependencies={["newPassword"]}
                    rules={[
                        { required: true, message: "请再次输入新密码" },
                        ({ getFieldValue }) => ({
                            validator(_, value) {
                                return !value || getFieldValue("newPassword") === value ? Promise.resolve() : Promise.reject(new Error("两次输入的新密码不一致"));
                            },
                        }),
                    ]}
                >
                    <Input.Password autoComplete="new-password" placeholder="再次输入新密码" />
                </Form.Item>
            </Form>
        </Modal>
    );
}
