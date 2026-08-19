import { useEffect, useMemo, useState } from "react";
import { App, Button } from "antd";
import { ArrowLeft, Save } from "lucide-react";
import { useNavigate, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";

import { AigcProjectTreePicker } from "@/components/aigc/aigc-project-tree-picker";
import { WorkspaceErrorState, WorkspaceLoadingState } from "@/components/layout/workspace-state";
import { WorkspacePage } from "@/components/layout/workspace-page";
import { listAvailableAigcProjectTree, type AigcProjectTreeNode } from "@/services/api/aigc";
import { saveRemoteUserDataNow } from "@/services/user-data-sync";
import { useCanvasStore } from "@/stores/canvas/use-canvas-store";

export default function CanvasSettingsPage() {
    const { message } = App.useApp();
    const navigate = useNavigate();
    const { id = "" } = useParams();
    const hydrated = useCanvasStore((state) => state.hydrated);
    const updateProject = useCanvasStore((state) => state.updateProject);
    const currentProject = useCanvasStore((state) => state.projects.find((project) => project.id === id));
    const aigcProjectsQuery = useQuery({ queryKey: ["aigc-projects", "available-tree"], queryFn: listAvailableAigcProjectTree });
    const [aigcProjectId, setAigcProjectId] = useState<number | undefined>();

    useEffect(() => {
        setAigcProjectId(currentProject?.aigcProjectId || undefined);
    }, [currentProject?.aigcProjectId, id]);

    useEffect(() => {
        const projects = flattenAigcProjectTree(aigcProjectsQuery.data?.projects || []).filter((item) => item.status === "启用");
        if (!projects.length) return;
        setAigcProjectId((current) => current && projects.some((item) => item.projectId === current) ? current : projects[0]?.projectId);
    }, [aigcProjectsQuery.data]);

    const dirty = useMemo(() => aigcProjectId !== (currentProject?.aigcProjectId || undefined), [aigcProjectId, currentProject?.aigcProjectId]);

    const save = async () => {
        if (!aigcProjectId) {
            message.warning("请选择业务项目");
            return;
        }
        updateProject(id, { aigcProjectId });
        try {
            await saveRemoteUserDataNow();
            message.success("画布业务项目已保存");
            navigate(`/canvas/${id}`);
        } catch (error) {
            message.error(error instanceof Error ? `画布业务项目保存失败：${error.message}` : "画布业务项目保存失败");
        }
    };

    if (!hydrated) return <WorkspacePage><WorkspaceLoadingState label="正在加载画布设置" detail="读取本地画布和业务项目树" /></WorkspacePage>;
    if (!currentProject) return <WorkspacePage><WorkspaceErrorState title="画布不可用" description="画布不存在、已被删除，或当前账号没有访问权限。" actionLabel="返回画布" onRetry={() => navigate("/canvas")} /></WorkspacePage>;

    return (
        <WorkspacePage className="canvas-settings-page" fluid>
            <div className="studio-band">
                <header className="flex min-h-14 items-center justify-between gap-3 border-b border-border/70 pb-3">
                    <div className="min-w-0">
                        <div className="flex min-w-0 items-center gap-2">
                            <Button type="text" className="!h-8 !w-8 !min-w-8 !p-0" icon={<ArrowLeft className="size-4" />} onClick={() => navigate(`/canvas/${id}`)} aria-label="返回画布" />
                            <h1 className="truncate text-lg font-semibold">画布设置</h1>
                        </div>
                        <p className="mt-1 text-xs text-foreground/45">只调整当前画布绑定的业务项目。</p>
                    </div>
                    <Button type={dirty ? "primary" : "default"} icon={<Save className="size-3.5" />} disabled={!dirty || !aigcProjectId} onClick={() => void save()}>
                        {dirty ? "保存设置" : "已保存"}
                    </Button>
                </header>

                <section className="mt-4 grid gap-4 rounded-lg border border-border/80 bg-background/70 p-4 lg:grid-cols-[minmax(0,1fr)_420px]">
                    <div className="min-w-0">
                        <h2 className="text-sm font-semibold">当前画布</h2>
                        <div className="mt-3 grid gap-2 text-sm text-foreground/60">
                            <InfoRow label="画布名称" value={currentProject.title || "未命名画布"} />
                            <InfoRow label="画布 ID" value={currentProject.id} />
                            <InfoRow label="故事项目" value={currentProject.projectId || "未绑定"} />
                        </div>
                    </div>
                    <div className="min-w-0">
                        <h2 className="text-sm font-semibold">业务项目</h2>
                        <p className="mt-1 text-xs text-foreground/45">一级项目和二级项目都可以选。</p>
                        <div className="mt-3">
                            <AigcProjectTreePicker
                                tree={aigcProjectsQuery.data?.projects || []}
                                loading={aigcProjectsQuery.isLoading}
                                error={aigcProjectsQuery.error instanceof Error ? aigcProjectsQuery.error.message : ""}
                                value={aigcProjectId}
                                required
                                onChange={setAigcProjectId}
                                placeholder="选择业务项目"
                                buttonClassName="creation-chat-control is-project w-full"
                            />
                        </div>
                    </div>
                </section>
            </div>
        </WorkspacePage>
    );
}

function InfoRow({ label, value }: { label: string; value: string }) {
    return <div className="grid grid-cols-[84px_minmax(0,1fr)] gap-3"><span className="text-xs text-foreground/38">{label}</span><span className="truncate text-xs text-foreground/70">{value}</span></div>;
}

function flattenAigcProjectTree(tree: AigcProjectTreeNode[]) {
    const result: AigcProjectTreeNode["project"][] = [];
    const visit = (nodes: AigcProjectTreeNode[]) => {
        for (const node of nodes) {
            result.push(node.project);
            if (node.children?.length) visit(node.children);
        }
    };
    visit(tree);
    return result;
}
