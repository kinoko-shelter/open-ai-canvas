const workspaceRouteLoaders = {
    "aigc-projects": () => import("@/pages/aigc-projects-page"),
    assets: () => import("@/pages/assets"),
    canvas: () => import("@/pages/canvas"),
    create: () => import("@/pages/create"),
    home: () => import("@/pages/home"),
    projects: () => import("@/pages/projects"),
    settings: () => import("@/pages/settings"),
    skills: () => import("@/pages/skills"),
    tasks: () => import("@/pages/tasks"),
    wallet: () => import("@/pages/wallet"),
};

export const loadAigcProjectsPage = workspaceRouteLoaders["aigc-projects"];
export const loadAssetsPage = workspaceRouteLoaders.assets;
export const loadCanvasPage = workspaceRouteLoaders.canvas;
export const loadCreatePage = workspaceRouteLoaders.create;
export const loadHomePage = workspaceRouteLoaders.home;
export const loadProjectsPage = workspaceRouteLoaders.projects;
export const loadSettingsPage = workspaceRouteLoaders.settings;
export const loadSkillsPage = workspaceRouteLoaders.skills;
export const loadTasksPage = workspaceRouteLoaders.tasks;
export const loadWalletPage = workspaceRouteLoaders.wallet;

export function preloadWorkspaceRoute(pathnameOrSlug: string) {
    const slug = pathnameOrSlug.replace(/^\//, "").split("/", 1)[0] as keyof typeof workspaceRouteLoaders;
    const load = workspaceRouteLoaders[slug];
    if (load) void load();
}
