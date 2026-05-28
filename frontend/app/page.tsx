"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Globe, LogOut, Search, Settings, Shield, Terminal } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import SubdomainList from "@/components/dns/subdomain-list";
import ApiTokens from "@/components/dns/api-tokens";
import AccountSettings from "@/components/dns/account-settings";
import DnsQueryTool from "@/components/dns/dns-query-tool";
import { api, clearToken, getToken, setToken } from "@/lib/api";
import { User } from "@/lib/mock-data";
import { toast } from "sonner";

export default function Dashboard() {
  const [activeTab, setActiveTab] = useState("subdomains");
  const [user, setUser] = useState<User | null>(null);
  const [email, setEmail] = useState("");
  const [nickname, setNickname] = useState("");
  const [password, setPassword] = useState("");
  const [needsSetup, setNeedsSetup] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const token = getToken();
    if (!token) {
      api.setupStatus()
        .then((status) => setNeedsSetup(!status.initialized))
        .catch(() => setNeedsSetup(false))
        .finally(() => setLoading(false));
      return;
    }
    api.me().then(setUser).catch(() => clearToken()).finally(() => setLoading(false));
  }, []);

  const login = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const result = await api.login(email, password);
      setToken(result.token);
      setUser(result.user);
      toast.success("Signed in.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Login failed");
    }
  };

  const setupAdmin = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const result = await api.setupAdmin(email, nickname, password);
      setToken(result.token);
      setUser(result.user);
      setNeedsSetup(false);
      toast.success("Admin account created.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Setup failed");
    }
  };

  const logout = () => {
    clearToken();
    setUser(null);
  };

  if (loading) return <div className="min-h-screen grid place-items-center text-slate-500">Loading...</div>;

  if (!user) {
    return (
      <div className="min-h-screen bg-slate-50 dark:bg-slate-950 grid place-items-center p-4">
        <Card className="w-full max-w-md">
          <CardHeader><CardTitle className="flex items-center gap-2"><Globe className="h-5 w-5" />{needsSetup ? "Create Admin Account" : "DNS Hub Login"}</CardTitle></CardHeader>
          <CardContent>
            <form onSubmit={needsSetup ? setupAdmin : login} className="space-y-4">
              <div className="space-y-2"><Label>Email</Label><Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required /></div>
              {needsSetup && <div className="space-y-2"><Label>Nickname</Label><Input value={nickname} onChange={(e) => setNickname(e.target.value)} placeholder="Administrator" /></div>}
              <div className="space-y-2"><Label>Password</Label><Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} required /></div>
              <Button type="submit" className="w-full">{needsSetup ? "Create Admin" : "Sign In"}</Button>
              {needsSetup && <p className="text-xs text-slate-500">No users exist yet. This account becomes the first administrator.</p>}
            </form>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
      <header className="border-b bg-white dark:bg-slate-900">
        <div className="container mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-2 text-primary font-semibold text-xl tracking-tight"><Globe className="h-6 w-6" /><span>DNS Hub</span></div>
          <div className="flex items-center gap-3">
            {user.role === "admin" && <Link href="/admin"><Button variant="outline" size="sm"><Shield className="h-4 w-4 mr-2" />Admin</Button></Link>}
            <div className="text-sm font-medium text-slate-600 dark:text-slate-300">{user.email}</div>
            <div className="px-3 py-1 bg-slate-100 dark:bg-slate-800 rounded-full text-xs font-semibold text-slate-500 dark:text-slate-400">{user.role.toUpperCase()}</div>
            <Button variant="ghost" size="icon" onClick={logout}><LogOut className="h-4 w-4" /></Button>
          </div>
        </div>
      </header>

      <main className="container mx-auto px-4 py-8">
        <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-6">
          <TabsList className="bg-white dark:bg-slate-900 border p-1 rounded-lg">
            <TabsTrigger value="subdomains" className="gap-2"><Globe className="h-4 w-4" />Subdomains</TabsTrigger>
            <TabsTrigger value="lookup" className="gap-2"><Search className="h-4 w-4" />DNS Lookup</TabsTrigger>
            <TabsTrigger value="tokens" className="gap-2"><Terminal className="h-4 w-4" />API Tokens</TabsTrigger>
            <TabsTrigger value="settings" className="gap-2"><Settings className="h-4 w-4" />Account</TabsTrigger>
          </TabsList>
          <TabsContent value="subdomains"><SubdomainList /></TabsContent>
          <TabsContent value="lookup"><DnsQueryTool /></TabsContent>
          <TabsContent value="tokens"><ApiTokens /></TabsContent>
          <TabsContent value="settings"><AccountSettings /></TabsContent>
        </Tabs>
      </main>
    </div>
  );
}
