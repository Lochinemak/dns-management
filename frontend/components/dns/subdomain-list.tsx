"use client";

import { useEffect, useState } from "react";
import dynamic from "next/dynamic";
import { Ban, Clock, Globe, Plus, Settings2, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { Domain, Subdomain } from "@/lib/mock-data";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter, CardHeader } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { toast } from "sonner";

const DnsManager = dynamic(() => import("./dns-manager"), {
  loading: () => (
    <div className="rounded-lg border border-dashed bg-white py-12 text-center text-sm text-slate-500 dark:bg-slate-900">
      Loading DNS records...
    </div>
  ),
});

export default function SubdomainList() {
  const [subdomains, setSubdomains] = useState<Subdomain[]>([]);
  const [domains, setDomains] = useState<Domain[]>([]);
  const [selectedSubdomain, setSelectedSubdomain] = useState<Subdomain | null>(null);
  const [isApplyOpen, setIsApplyOpen] = useState(false);
  const [newPrefix, setNewPrefix] = useState("");
  const [domainId, setDomainId] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [confirmSubdomainId, setConfirmSubdomainId] = useState("");
  const selectedDomain = domains.find((domain) => domain.id === domainId);

  const load = async () => {
    setLoading(true);
    try {
      setError("");
      const [subs, enabledDomains] = await Promise.all([api.subdomains(), api.enabledDomains()]);
      const nextSubdomains = Array.isArray(subs) ? subs : [];
      const nextDomains = Array.isArray(enabledDomains) ? enabledDomains : [];
      setSubdomains(nextSubdomains);
      setDomains(nextDomains);
      setDomainId(nextDomains[0]?.id || "");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to load subdomains";
      setError(message);
      toast.error(message);
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

  const handleApply = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setError("");
      const sub = await api.createSubdomain(domainId, newPrefix.trim().toLowerCase());
      setSubdomains([sub, ...subdomains]);
      setNewPrefix("");
      setIsApplyOpen(false);
      toast.success(`Requested ${sub.fullDomain}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to apply subdomain";
      setError(message);
      toast.error(message);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      setError("");
      await api.deleteSubdomain(id);
      setSubdomains(subdomains.filter((subdomain) => subdomain.id !== id));
      setConfirmSubdomainId("");
      setError("");
      toast.success("Subdomain deleted.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to delete subdomain";
      setError(message);
      toast.error(message);
    }
  };

  if (selectedSubdomain) {
    return (
      <DnsManager
        subdomain={selectedSubdomain}
        onBack={() => setSelectedSubdomain(null)}
        onDeleted={(id) => setSubdomains(subdomains.filter((subdomain) => subdomain.id !== id))}
      />
    );
  }
  const subdomainToDelete = subdomains.find((subdomain) => subdomain.id === confirmSubdomainId);

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">My Subdomains</h2>
          <p className="text-slate-500">Apply for third-level domains and manage approved DNS zones.</p>
        </div>
        <Dialog open={isApplyOpen} onOpenChange={setIsApplyOpen}>
          <Button className="gap-2" onClick={() => setIsApplyOpen(true)} disabled={domains.length === 0}>
            <Plus className="h-4 w-4" />
            Apply Subdomain
          </Button>
          <DialogContent>
            <DialogHeader><DialogTitle>Apply for a Subdomain</DialogTitle></DialogHeader>
            <form onSubmit={handleApply} className="space-y-4 pt-4">
              <div className="space-y-2">
                <Label>Root Domain</Label>
                <Select
                  value={selectedDomain?.name || ""}
                  onValueChange={(value) => setDomainId(domains.find((domain) => domain.name === value)?.id || "")}
                >
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>{domains.map((domain) => <SelectItem key={domain.id} value={domain.name}>{domain.name}</SelectItem>)}</SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Prefix</Label>
                <Input placeholder="blog" value={newPrefix} onChange={(e) => setNewPrefix(e.target.value)} required />
              </div>
              <DialogFooter><Button type="submit">Submit Request</Button></DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertTitle>Subdomain operation failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {subdomains.map((sub) => (
          <Card key={sub.id} className="flex flex-col">
            <CardHeader className="flex-row items-center justify-between pb-2">
              <div className="flex items-center gap-2 font-mono font-medium truncate text-lg">
                <Globe className="h-5 w-5 text-slate-400 shrink-0" />
                <span className="truncate">{sub.fullDomain}</span>
              </div>
              <StatusBadge status={sub.status} />
            </CardHeader>
            <CardContent className="flex-1 pb-4">
              <div className="text-xs text-slate-500 flex items-center gap-1.5 mt-2">
                <Clock className="h-3.5 w-3.5" />
                Created {new Date(sub.createdAt).toLocaleDateString()}
              </div>
              {sub.rejectReason && <p className="mt-2 text-sm text-red-500">{sub.rejectReason}</p>}
            </CardContent>
            <CardFooter className="pt-2 border-t bg-slate-50/50 dark:bg-slate-900/50 gap-2">
              <Button variant="secondary" className="flex-1 gap-2" onClick={() => setSelectedSubdomain(sub)} disabled={sub.status !== "active"}>
                {sub.status === "active" ? <><Settings2 className="h-4 w-4" />Manage DNS</> : <><Ban className="h-4 w-4" />Unavailable</>}
              </Button>
              <Button variant="outline" onClick={() => setConfirmSubdomainId(sub.id)} className="shrink-0 gap-2 text-red-500 hover:text-red-600 hover:bg-red-50">
                <Trash2 className="h-4 w-4" />
                Delete
              </Button>
            </CardFooter>
          </Card>
        ))}
        {(subdomains.length === 0 || loading) && (
          <div className="col-span-full py-12 text-center text-slate-500 border rounded-lg border-dashed">
            {loading ? "Loading..." : "No subdomains found."}
          </div>
        )}
      </div>
      <ConfirmDialog
        open={!!confirmSubdomainId}
        onOpenChange={(open) => !open && setConfirmSubdomainId("")}
        title="Delete Subdomain"
        description={`Delete ${subdomainToDelete?.fullDomain || "this subdomain"}? This removes the subdomain and its local DNS records from this service. This cannot be undone.`}
        confirmText="Delete Subdomain"
        destructive
        onConfirm={() => confirmSubdomainId && handleDelete(confirmSubdomainId)}
      />
    </div>
  );
}

function StatusBadge({ status }: { status: Subdomain["status"] }) {
  if (status === "active") return <Badge className="bg-emerald-500 shrink-0">Active</Badge>;
  if (status === "pending") return <Badge variant="secondary" className="shrink-0 text-amber-600 bg-amber-100">Pending</Badge>;
  if (status === "suspended") return <Badge variant="destructive" className="shrink-0">Suspended</Badge>;
  return <Badge variant="outline" className="text-red-500 shrink-0">Rejected</Badge>;
}
