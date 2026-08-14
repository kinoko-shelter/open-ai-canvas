import { useEffect, useMemo, useState } from "react";
import { App, Button, Result, Spin } from "antd";
import { ArrowLeft } from "lucide-react";
import { useNavigate, useSearchParams } from "react-router";

import { applyUserSession } from "@/lib/user-session";
import { getAuthSession, loginFromKOL, type AuthSessionPayload } from "@/services/api/auth";

const kolLoginRequests = new Map<string, Promise<AuthSessionPayload>>();
const kolLoginTimeoutMs = 30000;

export default function KOLCallbackPage() {
    const navigate = useNavigate();
    const [params] = useSearchParams();
    const { message } = App.useApp();
    const [error, setError] = useState("");
    const [status, setStatus] = useState("正在完成 KOL 登录...");
    const kolToken = useMemo(() => params.get("kol_token") || params.get("kolToken") || "", [params]);
    const next = safeNext(params.get("next"));

    useEffect(() => {
        let cancelled = false;
        async function completeLogin() {
            if (!kolToken) {
                setError("KOL 登录回调缺少 token");
                return;
            }
            try {
                setStatus("正在校验 KOL 登录凭证...");
                const payload = await withTimeout(completeKOLLogin(kolToken), kolLoginTimeoutMs, "KOL 登录校验超时，请重新登录");
                if (cancelled) return;
                setStatus("正在同步本系统会话...");
                await withTimeout(applyUserSession(payload), kolLoginTimeoutMs, "本系统会话同步超时，请刷新后重试");
                if (cancelled) return;
                message.success("登录成功");
                navigate(next, { replace: true });
            } catch (loginError) {
                if (!cancelled) setError(loginError instanceof Error ? loginError.message : "KOL 登录失败");
            }
        }
        void completeLogin();
        return () => {
            cancelled = true;
        };
    }, [kolToken, message, navigate, next]);

    if (error) {
        return (
            <Result
                status="error"
                title="KOL 登录失败"
                subTitle={error}
                extra={<Button icon={<ArrowLeft className="size-4" />} onClick={() => navigate("/login", { replace: true })}>返回登录</Button>}
            />
        );
    }

    return (
        <div className="grid min-h-56 place-items-center text-center">
            <div>
                <Spin />
                <p className="mt-4 text-sm text-white/58">{status}</p>
            </div>
        </div>
    );
}

function safeNext(value: string | null) {
    if (!value || !value.startsWith("/") || value.startsWith("//")) return "/create";
    return value;
}

function completeKOLLogin(kolToken: string) {
    const existing = kolLoginRequests.get(kolToken);
    if (existing) return existing;
    const request = loginFromKOL({ kolToken }).then(() => getAuthSession());
    kolLoginRequests.set(kolToken, request);
    return request;
}

function withTimeout<T>(promise: Promise<T>, timeoutMs: number, message: string) {
    return Promise.race<T>([
        promise,
        new Promise<T>((_, reject) => {
            window.setTimeout(() => reject(new Error(message)), timeoutMs);
        }),
    ]);
}
