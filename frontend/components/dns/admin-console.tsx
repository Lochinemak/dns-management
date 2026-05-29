"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Check, Eye, Globe, KeyRound, LockKeyhole, RotateCcw, Shield, Trash2, X } from "lucide-react";
import { api, clearToken } from "@/lib/api";
import { DnsRecord, Domain, ReservedSubdomain, Subdomain, User } from "@/lib/mock-data";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { toast } from "sonner";

export default function AdminConsole() {
  const [me, setMe] = useState<User | null>(null);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [pendingSubdomains, setPendingSubdomains] = useState<Subdomain[]>([]);
  const [appliedSubdomains, setAppliedSubdomains] = useState<Subdomain[]>([]);
  const [reservedSubdomains, setReservedSubdomains] = useState<ReservedSubdomain[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [domainName, setDomainName] = useState("");
  const [zoneId, setZoneId] = useState("");
  const [apiToken, setApiToken] = useState("");
  const [reservedDomainId, setReservedDomainId] = useState("");
  const [reservedPrefix, setReservedPrefix] = useState("");
  const [newUserEmail, setNewUserEmail] = useState("");
  const [newUserNickname, setNewUserNickname] = useState("");
  const [newUserRole, setNewUserRole] = useState<"user" | "admin">("user");
  const [newUserPassword, setNewUserPassword] = useState("");
  const [resetUser, setResetUser] = useState<User | null>(null);
  const [resetPassword, setResetPassword] = useState("");
  const [selectedSubdomain, setSelectedSubdomain] = useState<Subdomain | null>(null);
  const [selectedRecords, setSelectedRecords] = useState<DnsRecord[]>([]);
  const [recordsLoading, setRecordsLoading] = useState(false);
  const [loading, setLoading] = useState(true);

  const load = async () => {
    setLoading(true);
    try {
      const current = await api.me();
      setMe(current);
      if (current.role !== "admin") return;
      const [domainList, pendingList, appliedList, reservedList, userList] = await Promise.all([
        api.adminDomains(),
        api.adminSubdomains("pending"),
        api.adminSubdomains(),
        api.adminReservedSubdomains(),
        api.adminUsers(),
      ]);
      const nextDomains = Array.isArray(domainList) ? domainList : [];
      setDomains(nextDomains);
      setReservedDomainId((currentDomainId) => currentDomainId || nextDomains[0]?.id || "");
      setPendingSubdomains(Array.isArray(pendingList) ? pendingList : []);
      setAppliedSubdomains(Array.isArray(appliedList) ? appliedList : []);
      setReservedSubdomains(Array.isArray(reservedList) ? reservedList : []);
      setUsers(Array.isArray(userList) ? userList : []);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to load admin data");
      clearToken();
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      void load();
    }, 0);
    return () => window.clearTimeout(timeout);
  }, []);

  const addDomain = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const created = await api.createAdminDomain({ name: domainName, zoneId, apiToken, enabled: true });
      setDomains([created, ...domains]);
      setReservedDomainId((currentDomainId) => currentDomainId || created.id);
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

  const addReservedSubdomain = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const created = await api.createAdminReservedSubdomain({
        domainId: reservedDomainId,
        prefix: reservedPrefix.trim().toLowerCase(),
      });
      setReservedSubdomains([...reservedSubdomains, created].sort((a, b) => a.fullDomain.localeCompare(b.fullDomain)));
      setReservedPrefix("");
      toast.success("Reserved subdomain added.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to add reserved subdomain");
    }
  };

  const deleteReservedSubdomain = async (id: string) => {
    try {
      await api.deleteAdminReservedSubdomain(id);
      setReservedSubdomains(reservedSubdomains.filter((item) => item.id !== id));
      toast.success("Reserved subdomain deleted.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to delete reserved subdomain");
    }
  };

  const approve = async (id: string) => {
    await api.approveSubdomain(id);
    setPendingSubdomains(pendingSubdomains.filter((item) => item.id !== id));
    setAppliedSubdomains(appliedSubdomains.map((item) => item.id === id ? { ...item, status: "active" } : item));
    toast.success("Approved.");
  };

  const reject = async (id: string) => {
    await api.rejectSubdomain(id, "Rejected by administrator");
    setPendingSubdomains(pendingSubdomains.filter((item) => item.id !== id));
    setAppliedSubdomains(appliedSubdomains.map((item) => item.id === id ? { ...item, status: "rejected", rejectReason: "Rejected by administrator" } : item));
    toast.success("Rejected.");
  };

  const openSubdomainDetails = async (subdomain: Subdomain) => {
    setSelectedSubdomain(subdomain);
    setSelectedRecords([]);
    setRecordsLoading(true);
    try {
      setSelectedRecords(await api.records(subdomain.id));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to load DNS records");
    } finally {
      setRecordsLoading(false);
    }
  };

  const resetUserPassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!resetUser) return;
    try {
      await api.resetPassword(resetUser.id, resetPassword);
      setResetUser(null);
      setResetPassword("");
      toast.success("Password reset.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to reset password");
    }
  };

  const openResetPassword = (user: User) => {
    setResetUser(user);
    setResetPassword("");
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
          <CardHeader><CardTitle className="flex items-center gap-2"><LockKeyhole className="h-5 w-5" />Reserved Subdomains</CardTitle></CardHeader>
          <CardContent className="space-y-4">
            <form onSubmit={addReservedSubdomain} className="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-end">
              <div className="space-y-2">
                <Label>Root Domain</Label>
                <Select
                  value={domains.find((domain) => domain.id === reservedDomainId)?.name || ""}
                  onValueChange={(value) => value && setReservedDomainId(domains.find((domain) => domain.name === value)?.id || "")}
                >
                  <SelectTrigger><SelectValue placeholder="Select domain" /></SelectTrigger>
                  <SelectContent>{domains.map((domain) => <SelectItem key={domain.id} value={domain.name}>{domain.name}</SelectItem>)}</SelectContent>
                </Select>
              </div>
              <div className="space-y-2"><Label>Prefix</Label><Input placeholder="admin" value={reservedPrefix} onChange={(e) => setReservedPrefix(e.target.value)} required /></div>
              <Button type="submit" disabled={!reservedDomainId}>Reserve</Button>
            </form>
            <Table>
              <TableHeader><TableRow><TableHead>Reserved Subdomain</TableHead><TableHead>Created</TableHead><TableHead className="text-right">Action</TableHead></TableRow></TableHeader>
              <TableBody>
                {reservedSubdomains.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell className="font-mono">{item.fullDomain}</TableCell>
                    <TableCell>{new Date(item.createdAt).toLocaleString()}</TableCell>
                    <TableCell className="text-right"><Button size="icon" variant="ghost" onClick={() => deleteReservedSubdomain(item.id)} title="Delete reserved subdomain"><Trash2 className="h-4 w-4 text-red-500" /></Button></TableCell>
                  </TableRow>
                ))}
                {reservedSubdomains.length === 0 && <TableRow><TableCell colSpan={3} className="h-20 text-center text-slate-500">No reserved subdomains.</TableCell></TableRow>}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>Pending Subdomain Requests</CardTitle></CardHeader>
          <CardContent>
            <Table>
              <TableHeader><TableRow><TableHead>Subdomain</TableHead><TableHead>User</TableHead><TableHead>Created</TableHead><TableHead className="text-right">Actions</TableHead></TableRow></TableHeader>
              <TableBody>
                {pendingSubdomains.map((sub) => <TableRow key={sub.id}><TableCell className="font-mono">{sub.fullDomain}</TableCell><TableCell>{sub.ownerEmail}</TableCell><TableCell>{new Date(sub.createdAt).toLocaleString()}</TableCell><TableCell className="text-right space-x-2"><Button size="sm" onClick={() => approve(sub.id)}><Check className="h-4 w-4 mr-1" />Approve</Button><Button size="sm" variant="outline" onClick={() => reject(sub.id)}><X className="h-4 w-4 mr-1" />Reject</Button></TableCell></TableRow>)}
                {pendingSubdomains.length === 0 && <TableRow><TableCell colSpan={4} className="h-20 text-center text-slate-500">No pending requests.</TableCell></TableRow>}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>All Applied Subdomains</CardTitle></CardHeader>
          <CardContent>
            <Table>
              <TableHeader><TableRow><TableHead>Subdomain</TableHead><TableHead>User</TableHead><TableHead>Status</TableHead><TableHead>Created</TableHead><TableHead className="text-right">Details</TableHead></TableRow></TableHeader>
              <TableBody>
                {appliedSubdomains.map((sub) => (
                  <TableRow key={sub.id} className="cursor-pointer" onClick={() => openSubdomainDetails(sub)}>
                    <TableCell className="font-mono">{sub.fullDomain}</TableCell>
                    <TableCell>{sub.ownerEmail}</TableCell>
                    <TableCell><StatusBadge status={sub.status} /></TableCell>
                    <TableCell>{new Date(sub.createdAt).toLocaleString()}</TableCell>
                    <TableCell className="text-right"><Button size="icon" variant="ghost" title="View DNS records"><Eye className="h-4 w-4" /></Button></TableCell>
                  </TableRow>
                ))}
                {appliedSubdomains.length === 0 && <TableRow><TableCell colSpan={5} className="h-20 text-center text-slate-500">No applied subdomains.</TableCell></TableRow>}
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
            <Table>
              <TableHeader><TableRow><TableHead>Email</TableHead><TableHead>Role</TableHead><TableHead>Created</TableHead><TableHead className="text-right">Action</TableHead></TableRow></TableHeader>
              <TableBody>{users.map((user) => <TableRow key={user.id}><TableCell>{user.email}</TableCell><TableCell>{user.role}</TableCell><TableCell>{user.createdAt ? new Date(user.createdAt).toLocaleString() : ""}</TableCell><TableCell className="text-right"><Button size="sm" variant="outline" onClick={() => openResetPassword(user)}><RotateCcw className="h-4 w-4 mr-1" />Reset</Button></TableCell></TableRow>)}</TableBody>
            </Table>
          </CardContent>
        </Card>
      </main>

      <Dialog open={!!selectedSubdomain} onOpenChange={(open) => !open && setSelectedSubdomain(null)}>
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>{selectedSubdomain?.fullDomain}</DialogTitle>
            <DialogDescription>DNS records for this applied subdomain.</DialogDescription>
          </DialogHeader>
          <div className="border rounded-lg overflow-hidden">
            <Table>
              <TableHeader><TableRow><TableHead>Type</TableHead><TableHead>Name</TableHead><TableHead>Content</TableHead><TableHead>TTL</TableHead><TableHead>Proxy</TableHead></TableRow></TableHeader>
              <TableBody>
                {selectedRecords.map((record) => (
                  <TableRow key={record.id}>
                    <TableCell className="font-semibold">{record.type}</TableCell>
                    <TableCell className="font-mono text-sm">{formatRecordName(record.name, selectedSubdomain?.fullDomain || "")}</TableCell>
                    <TableCell className="font-mono text-sm break-all">{record.content}</TableCell>
                    <TableCell>{record.ttl === 1 ? "Auto" : `${record.ttl}s`}</TableCell>
                    <TableCell>{record.proxied ? "On" : "Off"}</TableCell>
                  </TableRow>
                ))}
                {(selectedRecords.length === 0 || recordsLoading) && <TableRow><TableCell colSpan={5} className="h-20 text-center text-slate-500">{recordsLoading ? "Loading..." : "No DNS records found."}</TableCell></TableRow>}
              </TableBody>
            </Table>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={!!resetUser} onOpenChange={(open) => {
        if (!open) {
          setResetUser(null);
          setResetPassword("");
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reset Password</DialogTitle>
            <DialogDescription>{resetUser ? `Set a new password for ${resetUser.email}.` : "Set a new password."}</DialogDescription>
          </DialogHeader>
          <form onSubmit={resetUserPassword} className="space-y-4">
            <div className="space-y-2">
              <Label>New Password</Label>
              <Input type="password" minLength={8} value={resetPassword} onChange={(e) => setResetPassword(e.target.value)} required />
            </div>
            <DialogFooter><Button type="submit">Reset Password</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function StatusBadge({ status }: { status: Subdomain["status"] }) {
  if (status === "active") return <Badge className="bg-emerald-500 shrink-0">Active</Badge>;
  if (status === "pending") return <Badge variant="secondary" className="shrink-0 text-amber-600 bg-amber-100">Pending</Badge>;
  if (status === "suspended") return <Badge variant="destructive" className="shrink-0">Suspended</Badge>;
  return <Badge variant="outline" className="text-red-500 shrink-0">Rejected</Badge>;
}

function formatRecordName(name: string, fullDomain: string) {
  if (!fullDomain) return name;
  return name === "@" ? fullDomain : `${name}.${fullDomain}`;
}
