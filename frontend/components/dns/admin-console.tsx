"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Check, Globe, KeyRound, RotateCcw, Shield, X } from "lucide-react";
import { api, clearToken } from "@/lib/api";
import { Domain, Subdomain, User } from "@/lib/mock-data";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { toast } from "sonner";

export default function AdminConsole() {
  const [me, setMe] = useState<User | null>(null);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [subdomains, setSubdomains] = useState<Subdomain[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [domainName, setDomainName] = useState("");
  const [zoneId, setZoneId] = useState("");
  const [apiToken, setApiToken] = useState("");
  const [newUserEmail, setNewUserEmail] = useState("");
  const [newUserNickname, setNewUserNickname] = useState("");
  const [newUserRole, setNewUserRole] = useState<"user" | "admin">("user");
  const [newUserPassword, setNewUserPassword] = useState("");
  const [resetPassword, setResetPassword] = useState("ChangeMe123");
  const [loading, setLoading] = useState(true);

  const load = async () => {
    setLoading(true);
    try {
      const current = await api.me();
      setMe(current);
      if (current.role !== "admin") return;
      const [domainList, requestList, userList] = await Promise.all([api.adminDomains(), api.adminSubdomains("pending"), api.adminUsers()]);
      setDomains(Array.isArray(domainList) ? domainList : []);
      setSubdomains(Array.isArray(requestList) ? requestList : []);
      setUsers(Array.isArray(userList) ? userList : []);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to load admin data");
      clearToken();
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
  }, []);

  const addDomain = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const created = await api.createAdminDomain({ name: domainName, zoneId, apiToken, enabled: true });
      setDomains([created, ...domains]);
      setDomainName("");
      setZoneId("");
      setApiToken("");
      toast.success("Domain added.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to add domain");
    }
  };

  const toggleDomain = async (domain: Domain) => {
    await api.updateAdminDomain(domain.id, { enabled: !domain.enabled });
    setDomains(domains.map((item) => item.id === domain.id ? { ...item, enabled: !item.enabled } : item));
  };

  const approve = async (id: string) => {
    await api.approveSubdomain(id);
    setSubdomains(subdomains.filter((item) => item.id !== id));
    toast.success("Approved.");
  };

  const reject = async (id: string) => {
    await api.rejectSubdomain(id, "Rejected by administrator");
    setSubdomains(subdomains.filter((item) => item.id !== id));
    toast.success("Rejected.");
  };

  const resetUserPassword = async (id: string) => {
    await api.resetPassword(id, resetPassword);
    toast.success("Password reset.");
  };

  const addUser = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const created = await api.createUser({
        email: newUserEmail,
        nickname: newUserNickname,
        role: newUserRole,
        password: newUserPassword,
      });
      setUsers([created, ...users]);
      setNewUserEmail("");
      setNewUserNickname("");
      setNewUserRole("user");
      setNewUserPassword("");
      toast.success("User added.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to add user");
    }
  };

  if (loading) return <div className="min-h-screen grid place-items-center text-slate-500">Loading...</div>;
  if (!me || me.role !== "admin") {
    return (
      <div className="min-h-screen grid place-items-center bg-slate-50 p-4">
        <Card className="max-w-md w-full"><CardHeader><CardTitle>Access denied</CardTitle></CardHeader><CardContent><Link href="/"><Button>Back to dashboard</Button></Link></CardContent></Card>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
      <header className="border-b bg-white dark:bg-slate-900">
        <div className="container mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-2 font-semibold text-xl"><Shield className="h-6 w-6" />Admin Console</div>
          <Link href="/"><Button variant="outline">Dashboard</Button></Link>
        </div>
      </header>
      <main className="container mx-auto px-4 py-8 space-y-6">
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><Globe className="h-5 w-5" />Root Domains</CardTitle></CardHeader>
          <CardContent className="space-y-4">
            <form onSubmit={addDomain} className="grid gap-4 md:grid-cols-4 items-end">
              <div className="space-y-2"><Label>Domain</Label><Input value={domainName} onChange={(e) => setDomainName(e.target.value)} placeholder="example.com" required /></div>
              <div className="space-y-2"><Label>Zone ID</Label><Input value={zoneId} onChange={(e) => setZoneId(e.target.value)} required /></div>
              <div className="space-y-2"><Label>Cloudflare API Token</Label><Input type="password" value={apiToken} onChange={(e) => setApiToken(e.target.value)} required /></div>
              <Button type="submit">Add Domain</Button>
            </form>
            <Table>
              <TableHeader><TableRow><TableHead>Domain</TableHead><TableHead>Zone ID</TableHead><TableHead>Token</TableHead><TableHead>Status</TableHead><TableHead /></TableRow></TableHeader>
              <TableBody>{domains.map((domain) => <TableRow key={domain.id}><TableCell>{domain.name}</TableCell><TableCell className="font-mono">{domain.zoneId}</TableCell><TableCell>{domain.tokenMasked}</TableCell><TableCell>{domain.enabled ? "Enabled" : "Disabled"}</TableCell><TableCell className="text-right"><Button variant="outline" size="sm" onClick={() => toggleDomain(domain)}>{domain.enabled ? "Disable" : "Enable"}</Button></TableCell></TableRow>)}</TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Pending Subdomain Requests</CardTitle></CardHeader>
          <CardContent>
            <Table>
              <TableHeader><TableRow><TableHead>Subdomain</TableHead><TableHead>User</TableHead><TableHead>Created</TableHead><TableHead className="text-right">Actions</TableHead></TableRow></TableHeader>
              <TableBody>
                {subdomains.map((sub) => <TableRow key={sub.id}><TableCell className="font-mono">{sub.fullDomain}</TableCell><TableCell>{sub.ownerEmail}</TableCell><TableCell>{new Date(sub.createdAt).toLocaleString()}</TableCell><TableCell className="text-right space-x-2"><Button size="sm" onClick={() => approve(sub.id)}><Check className="h-4 w-4 mr-1" />Approve</Button><Button size="sm" variant="outline" onClick={() => reject(sub.id)}><X className="h-4 w-4 mr-1" />Reject</Button></TableCell></TableRow>)}
                {subdomains.length === 0 && <TableRow><TableCell colSpan={4} className="h-20 text-center text-slate-500">No pending requests.</TableCell></TableRow>}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><KeyRound className="h-5 w-5" />Users</CardTitle></CardHeader>
          <CardContent className="space-y-4">
            <form onSubmit={addUser} className="grid gap-4 md:grid-cols-5 items-end">
              <div className="space-y-2"><Label>Email</Label><Input type="email" value={newUserEmail} onChange={(e) => setNewUserEmail(e.target.value)} required /></div>
              <div className="space-y-2"><Label>Nickname</Label><Input value={newUserNickname} onChange={(e) => setNewUserNickname(e.target.value)} placeholder="Display name" /></div>
              <div className="space-y-2">
                <Label>Role</Label>
                <Select value={newUserRole} onValueChange={(value) => value && setNewUserRole(value as "user" | "admin")}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="user">user</SelectItem>
                    <SelectItem value="admin">admin</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2"><Label>Password</Label><Input type="password" minLength={8} value={newUserPassword} onChange={(e) => setNewUserPassword(e.target.value)} required /></div>
              <Button type="submit">Add User</Button>
            </form>
            <div className="max-w-xs space-y-2"><Label>Reset Password Value</Label><Input value={resetPassword} onChange={(e) => setResetPassword(e.target.value)} /></div>
            <Table>
              <TableHeader><TableRow><TableHead>Email</TableHead><TableHead>Role</TableHead><TableHead>Created</TableHead><TableHead className="text-right">Action</TableHead></TableRow></TableHeader>
              <TableBody>{users.map((user) => <TableRow key={user.id}><TableCell>{user.email}</TableCell><TableCell>{user.role}</TableCell><TableCell>{user.createdAt ? new Date(user.createdAt).toLocaleString() : ""}</TableCell><TableCell className="text-right"><Button size="sm" variant="outline" onClick={() => resetUserPassword(user.id)}><RotateCcw className="h-4 w-4 mr-1" />Reset</Button></TableCell></TableRow>)}</TableBody>
            </Table>
          </CardContent>
        </Card>
      </main>
    </div>
  );
}
