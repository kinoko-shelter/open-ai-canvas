import { apiClient, request } from "@/services/api/request";

const api = apiClient;

export type AigcDepartment = {
    deptId: number;
    parentId: number;
    name: string;
    status: "启用" | "禁用";
    remark?: string;
    createdBy?: string;
    updatedBy?: string;
    createdAt: string;
    updatedAt: string;
};

export type AigcProject = {
    projectId: number;
    parentId?: number;
    deptId?: number;
    firstCategoryId?: number;
    firstCategoryName?: string;
    projectName: string;
    projectNameOuter?: string;
    projectType: string;
    projectDesc?: string;
    status: "启用" | "禁用";
    level: 1 | 2;
    remark?: string;
    createdBy?: string;
    updatedBy?: string;
    createdAt: string;
    updatedAt: string;
};

export type AigcProjectTreeNode = {
    project: AigcProject;
    children?: AigcProjectTreeNode[];
};

export type AigcProjectInput = Pick<AigcProject, "projectName" | "projectNameOuter" | "projectType" | "projectDesc" | "status" | "level" | "parentId" | "deptId" | "remark">;

export function listAigcDepartments(params: { keyword?: string } = {}) {
    return request<{ departments: AigcDepartment[] }>(api.get("/aigc/departments", { params }));
}

export function createAigcDepartment(input: { name: string; status: "启用" | "禁用"; remark?: string }) {
    return request<{ department: AigcDepartment }>(api.post("/aigc/departments", input));
}

export function updateAigcDepartment(id: number, input: { name: string; status: "启用" | "禁用"; remark?: string }) {
    return request<{ department: AigcDepartment }>(api.patch(`/aigc/departments/${encodeURIComponent(id)}`, input));
}

export function listAigcProjects(params: { keyword?: string; status?: string; level?: string; parentId?: number; page?: number; limit?: number } = {}) {
    return request<{ projects: AigcProject[]; total: number; page: number; limit: number }>(api.get("/aigc/projects", { params }));
}

export function listAvailableAigcProjects() {
    return request<{ projects: AigcProject[] }>(api.get("/aigc/projects/available"));
}

export function listAvailableAigcProjectTree() {
    return request<{ projects: AigcProjectTreeNode[] }>(api.get("/aigc/projects/available/tree"));
}

export function createAigcProject(input: AigcProjectInput) {
    return request<{ project: AigcProject }>(api.post("/aigc/projects", input));
}

export function updateAigcProject(id: number, input: AigcProjectInput) {
    return request<{ project: AigcProject }>(api.patch(`/aigc/projects/${encodeURIComponent(id)}`, input));
}

export function aigcDepartmentOptions(departments: AigcDepartment[]) {
    return departments.map((team) => ({ label: `${team.name} · ID: ${team.deptId}`, value: team.deptId, disabled: team.status === "禁用" }));
}

export function aigcProjectLevelLabel(level?: number) {
    return level === 1 ? "一级" : level === 2 ? "二级" : "--";
}

export function aigcRoleLabel(role?: string) {
    const labels: Record<string, string> = { admin: "管理员", user: "普通用户", operations_manager: "运营管理", team_lead: "团队主管", team_member: "团队成员" };
    return labels[role || ""] || role || "--";
}

export const aigcRoleOptions = [
    { label: "管理员", value: "admin" },
    { label: "普通用户", value: "user" },
    { label: "运营管理", value: "operations_manager" },
    { label: "团队主管", value: "team_lead" },
    { label: "团队成员", value: "team_member" },
];
