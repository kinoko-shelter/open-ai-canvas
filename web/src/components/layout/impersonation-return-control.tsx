import { App, Button } from "antd";
import { RotateCcw, ShieldCheck } from "lucide-react";
import { useState } from "react";

import { applyUserSession } from "@/lib/user-session";
import { exitUserImpersonation, getAuthSession } from "@/services/api/auth";
import { useUserStore } from "@/stores/use-user-store";

export function ImpersonationReturnControl() {
    const impersonation = useUserStore((state) => state.impersonation);
    const { message } = App.useApp();
    const [exiting, setExiting] = useState(false);

    if (!impersonation) return null;

    const actorName = impersonation.actorDisplayName || impersonation.actorUsername;
    const exitImpersonation = async () => {
        setExiting(true);
        try {
            await exitUserImpersonation();
            await applyUserSession(await getAuthSession());
            message.success("已返回管理员账号");
            window.location.replace("/admin/users");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "返回管理员账号失败");
        } finally {
            setExiting(false);
        }
    };

    return (
        <div className="pointer-events-none fixed right-3 top-3 z-50">
            <div className="pointer-events-auto flex items-center gap-3 rounded-md border border-border bg-background/95 px-3 py-2 shadow-lg backdrop-blur">
                <ShieldCheck className="size-4 shrink-0 text-foreground/65" />
                <span className="max-w-52 truncate text-xs text-foreground/70">正在以用户身份查看，来自 {actorName}</span>
                <Button size="small" icon={<RotateCcw className="size-3.5" />} loading={exiting} onClick={() => void exitImpersonation()}>返回管理员</Button>
            </div>
        </div>
    );
}
