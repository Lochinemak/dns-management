"use client";

import type { ApiToken, DnsRecord, DnsRecordType, Domain, Subdomain, User } from "@/lib/mock-data";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL || "/api/v1";
const TOKEN_KEY = "dns_hub_token";

export type LoginResponse = {
  token: string;
  user: User;
};

export type SetupStatus = {
  initialized: boolean;
};

export type TokenCreateResponse = ApiToken & {
  token?: string;
};

export type DnsQueryAnswer = {
  name: string;
  type: number;
  TTL: number;
  data: string;
};

export type DnsQueryResponse = {
  Status: number;
  TC: boolean;
  RD: boolean;
  RA: boolean;
  AD: boolean;
  CD: boolean;
  Question?: Array<{ name: string; type: number }>;
  Answer?: DnsQueryAnswer[];
  Comment?: string;
};

export function getToken() {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Content-Type", "application/json");
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(`${API_BASE}${path}`, { ...options, headers });
  if (response.status === 204) return undefined as T;
  const data = await response.json().catch(() => null);
  if (!response.ok) {
    throw new Error(data?.error || `Request failed with ${response.status}`);
  }
  return data as T;
}

export const api = {
  setupStatus: () => apiFetch<SetupStatus>("/setup/status", { method: "GET" }),
  setupAdmin: (email: string, nickname: string, password: string) =>
    apiFetch<LoginResponse>("/setup/admin", {
      method: "POST",
      body: JSON.stringify({ email, nickname, password }),
    }),
  login: (email: string, password: string) =>
    apiFetch<LoginResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    }),
  me: () => apiFetch<User>("/me"),
  changePassword: (oldPassword: string, newPassword: string) =>
    apiFetch<{ status: string }>("/auth/change-password", {
      method: "POST",
      body: JSON.stringify({ oldPassword, newPassword }),
    }),
  enabledDomains: () => apiFetch<Domain[]>("/domains/enabled"),
  subdomains: () => apiFetch<Subdomain[]>("/subdomains"),
  createSubdomain: (domainId: string, prefix: string) =>
    apiFetch<Subdomain>("/subdomains", {
      method: "POST",
      body: JSON.stringify({ domainId, prefix }),
    }),
  deleteSubdomain: (id: string) => apiFetch<void>(`/subdomains/${id}`, { method: "DELETE" }),
  records: (subdomainId: string) => apiFetch<DnsRecord[]>(`/subdomains/${subdomainId}/records`),
  syncRecords: (subdomainId: string) =>
    apiFetch<DnsRecord[]>(`/subdomains/${subdomainId}/records/sync`, {
      method: "POST",
      body: "{}",
    }),
  createRecord: (subdomainId: string, record: Omit<DnsRecord, "id" | "subdomainId">) =>
    apiFetch<DnsRecord>(`/subdomains/${subdomainId}/records`, {
      method: "POST",
      body: JSON.stringify(record),
    }),
  deleteRecord: (subdomainId: string, recordId: string) =>
    apiFetch<void>(`/subdomains/${subdomainId}/records/${recordId}`, { method: "DELETE" }),
  tokens: () => apiFetch<ApiToken[]>("/tokens"),
  createToken: (name: string) =>
    apiFetch<TokenCreateResponse>("/tokens", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  deleteToken: (id: string) => apiFetch<void>(`/tokens/${id}`, { method: "DELETE" }),
  rotateToken: (id: string) =>
    apiFetch<TokenCreateResponse>(`/tokens/${id}/rotate`, {
      method: "POST",
      body: "{}",
    }),
  dnsQuery: (name: string, type: DnsRecordType) =>
    apiFetch<DnsQueryResponse>(`/dns-query?name=${encodeURIComponent(name)}&type=${encodeURIComponent(type)}`, {
      method: "GET",
    }),
  adminDomains: () => apiFetch<Domain[]>("/admin/domains"),
  createAdminDomain: (body: { name: string; zoneId: string; apiToken: string; enabled: boolean }) =>
    apiFetch<Domain>("/admin/domains", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateAdminDomain: (id: string, body: Partial<{ name: string; zoneId: string; apiToken: string; enabled: boolean }>) =>
    apiFetch<{ status: string }>(`/admin/domains/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    }),
  deleteAdminDomain: (id: string) => apiFetch<void>(`/admin/domains/${id}`, { method: "DELETE" }),
  adminSubdomains: (status = "") =>
    apiFetch<Subdomain[]>(`/admin/subdomains${status ? `?status=${encodeURIComponent(status)}` : ""}`),
  approveSubdomain: (id: string) =>
    apiFetch<{ status: string }>(`/admin/subdomains/${id}/approve`, { method: "POST", body: "{}" }),
  rejectSubdomain: (id: string, reason: string) =>
    apiFetch<{ status: string }>(`/admin/subdomains/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),
  adminUsers: () => apiFetch<User[]>("/admin/users"),
  createUser: (body: { email: string; nickname: string; role: "user" | "admin"; password: string }) =>
    apiFetch<User>("/admin/users", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  resetPassword: (id: string, newPassword: string) =>
    apiFetch<{ status: string }>(`/admin/users/${id}/reset-password`, {
      method: "POST",
      body: JSON.stringify({ newPassword }),
    }),
};
