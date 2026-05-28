"use client";

import { useEffect, useState } from "react";
import { ArrowLeft, Plus, Trash2, Cloud, CloudOff, RefreshCw, Pencil } from "lucide-react";
import { api } from "@/lib/api";
import { DnsRecord, Subdomain } from "@/lib/mock-data";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { toast } from "sonner";

export default function DnsManager({ subdomain, onBack, onDeleted }: { subdomain: Subdomain; onBack: () => void; onDeleted?: (id: string) => void }) {
  const [records, setRecords] = useState<DnsRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [isAddOpen, setIsAddOpen] = useState(false);
  const [editingRecord, setEditingRecord] = useState<DnsRecord | null>(null);
  const [type, setType] = useState<DnsRecord["type"]>("A");
  const [name, setName] = useState("@");
  const [content, setContent] = useState("");
  const [ttl, setTtl] = useState(1);
  const [proxied, setProxied] = useState(false);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [formError, setFormError] = useState("");
  const [pageError, setPageError] = useState("");
  const [confirmRecordId, setConfirmRecordId] = useState("");
  const [confirmSubdomainDelete, setConfirmSubdomainDelete] = useState(false);

  useEffect(() => {
    let active = true;
    async function loadRecords() {
      setLoading(true);
      try {
        const nextRecords = await api.records(subdomain.id);
        if (active) setRecords(Array.isArray(nextRecords) ? nextRecords : []);
      } catch (error) {
        toast.error(error instanceof Error ? error.message : "Failed to load DNS records");
      } finally {
        if (active) setLoading(false);
      }
    }

    void loadRecords();
    return () => {
      active = false;
    };
  }, [subdomain.id]);

  const resetForm = () => {
    setType("A");
    setName("@");
    setContent("");
    setTtl(1);
    setProxied(false);
    setFormError("");
  };

  const setRecordType = (value: DnsRecord["type"]) => {
    setType(value);
    if (!canProxy(value)) setProxied(false);
  };

  const openEdit = (record: DnsRecord) => {
    setEditingRecord(record);
    setType(record.type);
    setName(record.name);
    setContent(record.content);
    setTtl(record.ttl);
    setProxied(record.proxied);
    setFormError("");
  };

  const handleAddSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError("");
    const validationError = validateRecordContent(type, content);
    if (validationError) {
      setFormError(validationError);
      setSaving(false);
      return;
    }
    try {
      const record = await api.createRecord(subdomain.id, {
        type,
        name: name.trim() || "@",
        content: content.trim(),
        ttl,
        proxied,
      });
      setRecords([record, ...records]);
      setIsAddOpen(false);
      resetForm();
      toast.success("DNS record added.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to add DNS record";
      setFormError(message);
      toast.error(message);
    } finally {
      setSaving(false);
    }
  };

  const handleEditSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingRecord) return;
    setSaving(true);
    setFormError("");
    const validationError = validateRecordContent(type, content);
    if (validationError) {
      setFormError(validationError);
      setSaving(false);
      return;
    }
    try {
      const updated = await api.updateRecord(subdomain.id, editingRecord.id, {
        type,
        name: name.trim() || "@",
        content: content.trim(),
        ttl,
        proxied,
      });
      setRecords(records.map((record) => record.id === updated.id ? updated : record));
      setEditingRecord(null);
      resetForm();
      toast.success("DNS record updated.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to update DNS record";
      setFormError(message);
      toast.error(message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      setPageError("");
      await api.deleteRecord(subdomain.id, id);
      setRecords(records.filter((r) => r.id !== id));
      setConfirmRecordId("");
      toast.success("Record deleted.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to delete record";
      setPageError(message);
      toast.error(message);
    }
  };

  const handleDeleteSubdomain = async () => {
    try {
      setPageError("");
      await api.deleteSubdomain(subdomain.id);
      onDeleted?.(subdomain.id);
      setConfirmSubdomainDelete(false);
      toast.success("Subdomain deleted.");
      onBack();
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to delete subdomain";
      setPageError(message);
      toast.error(message);
    }
  };

  const handleSync = async () => {
    setSyncing(true);
    setPageError("");
    try {
      const syncedRecords = await api.syncRecords(subdomain.id);
      setRecords(Array.isArray(syncedRecords) ? syncedRecords : []);
      toast.success("DNS records synced from Cloudflare.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to sync DNS records";
      setPageError(message);
      toast.error(message);
    } finally {
      setSyncing(false);
    }
  };

  const getFullRecordName = (recordName: string) => (recordName === "@" ? subdomain.fullDomain : `${recordName}.${subdomain.fullDomain}`);
  const ttlLabel = (value: number) => (value === 1 ? "Auto" : value === 60 ? "1 min" : "1 hr");
  const ttlValue = (value: string | null) => (value === "1 min" ? 60 : value === "1 hr" ? 3600 : 1);
  const recordToDelete = records.find((record) => record.id === confirmRecordId);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <Button variant="ghost" size="icon" onClick={onBack}>
            <ArrowLeft className="h-5 w-5" />
          </Button>
          <div>
            <h2 className="text-2xl font-semibold tracking-tight">{subdomain.fullDomain}</h2>
            <p className="text-slate-500">Manage DNS records for this approved subdomain.</p>
          </div>
        </div>
        <Button
          variant="outline"
          className="gap-2 text-red-500 hover:text-red-600 hover:bg-red-50"
          onClick={() => setConfirmSubdomainDelete(true)}
          disabled={records.length > 0 || loading}
          title={records.length > 0 ? "Delete all DNS records first" : "Delete this subdomain"}
        >
          <Trash2 className="h-4 w-4" />
          Delete Subdomain
        </Button>
      </div>

      <div className="flex justify-between items-center border-t pt-6">
        <h3 className="font-semibold text-lg">DNS Records</h3>
        <div className="flex items-center gap-2">
          <Button variant="outline" className="gap-2" onClick={handleSync} disabled={syncing}>
            <RefreshCw className={`h-4 w-4 ${syncing ? "animate-spin" : ""}`} />
            {syncing ? "Syncing..." : "Sync"}
          </Button>
          <Dialog open={isAddOpen} onOpenChange={(open) => { setIsAddOpen(open); if (!open) resetForm(); }}>
          <Button className="gap-2" onClick={() => setIsAddOpen(true)}>
            <Plus className="h-4 w-4" />
            Add Record
          </Button>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Add DNS Record</DialogTitle>
            </DialogHeader>
            <form onSubmit={handleAddSubmit} className="space-y-4 pt-4">
              {formError && (
                <Alert variant="destructive">
                  <AlertTitle>Could not save DNS record</AlertTitle>
                  <AlertDescription>{formError}</AlertDescription>
                </Alert>
              )}
              <div className="grid grid-cols-4 gap-4">
                <div className="space-y-2">
                  <Label>Type</Label>
                  <Select value={type} onValueChange={(value) => value && setRecordType(value as DnsRecord["type"])}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {["A", "AAAA", "CNAME", "TXT", "MX", "NS"].map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}
                    </SelectContent>
                  </Select>
                </div>
                <div className="col-span-3 space-y-2">
                  <Label>Name</Label>
                  <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="@" required />
                </div>
              </div>
              <div className="space-y-2">
                <Label>Content</Label>
                <Input value={content} onChange={(e) => setContent(e.target.value)} placeholder="192.168.1.1" required />
              </div>
              <div className="flex items-center justify-between">
                <Button type="button" variant={proxied ? "default" : "outline"} className={`gap-2 h-8 ${proxied ? "bg-orange-500 hover:bg-orange-600" : ""}`} onClick={() => setProxied(!proxied)} disabled={!canProxy(type)}>
                  {proxied ? <Cloud className="h-4 w-4" /> : <CloudOff className="h-4 w-4" />}
                  {proxied ? "Proxied" : "DNS only"}
                </Button>
                <Select value={ttlLabel(ttl)} onValueChange={(value) => setTtl(ttlValue(value))}>
                  <SelectTrigger className="w-[120px]"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="Auto">Auto</SelectItem>
                    <SelectItem value="1 min">1 min</SelectItem>
                    <SelectItem value="1 hr">1 hr</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <DialogFooter>
                <Button type="submit" disabled={saving}>{saving ? "Saving..." : "Save"}</Button>
              </DialogFooter>
            </form>
          </DialogContent>
          </Dialog>
        </div>
      </div>

      <Dialog open={!!editingRecord} onOpenChange={(open) => { if (!open) { setEditingRecord(null); resetForm(); } }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit DNS Record</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleEditSubmit} className="space-y-4 pt-4">
            {formError && (
              <Alert variant="destructive">
                <AlertTitle>Could not update DNS record</AlertTitle>
                <AlertDescription>{formError}</AlertDescription>
              </Alert>
            )}
            <div className="grid grid-cols-4 gap-4">
              <div className="space-y-2">
                <Label>Type</Label>
                <Select value={type} onValueChange={(value) => value && setRecordType(value as DnsRecord["type"])}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {["A", "AAAA", "CNAME", "TXT", "MX", "NS"].map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="col-span-3 space-y-2">
                <Label>Name</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="@" required />
              </div>
            </div>
            <div className="space-y-2">
              <Label>Content</Label>
              <Input value={content} onChange={(e) => setContent(e.target.value)} placeholder="192.168.1.1" required />
            </div>
            <div className="flex items-center justify-between">
              <Button type="button" variant={proxied ? "default" : "outline"} className={`gap-2 h-8 ${proxied ? "bg-orange-500 hover:bg-orange-600" : ""}`} onClick={() => setProxied(!proxied)} disabled={!canProxy(type)}>
                {proxied ? <Cloud className="h-4 w-4" /> : <CloudOff className="h-4 w-4" />}
                {proxied ? "Proxied" : "DNS only"}
              </Button>
              <Select value={ttlLabel(ttl)} onValueChange={(value) => setTtl(ttlValue(value))}>
                <SelectTrigger className="w-[120px]"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="Auto">Auto</SelectItem>
                  <SelectItem value="1 min">1 min</SelectItem>
                  <SelectItem value="1 hr">1 hr</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <DialogFooter>
              <Button type="submit" disabled={saving}>{saving ? "Saving..." : "Save Changes"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {pageError && (
        <Alert variant="destructive">
          <AlertTitle>DNS operation failed</AlertTitle>
          <AlertDescription>{pageError}</AlertDescription>
        </Alert>
      )}

      <div className="border rounded-lg bg-white dark:bg-slate-900 shadow-sm overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Type</TableHead>
              <TableHead>Name</TableHead>
              <TableHead>Content</TableHead>
              <TableHead>Proxy</TableHead>
              <TableHead>TTL</TableHead>
              <TableHead className="w-[96px]" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {records.map((record) => (
              <TableRow key={record.id}>
                <TableCell className="font-semibold">{record.type}</TableCell>
                <TableCell className="font-mono text-sm">{getFullRecordName(record.name)}</TableCell>
                <TableCell className="font-mono text-sm">{record.content}</TableCell>
                <TableCell>{record.proxied ? <Cloud className="h-5 w-5 text-orange-500" /> : <CloudOff className="h-5 w-5 text-slate-300" />}</TableCell>
                <TableCell>{record.ttl === 1 ? "Auto" : `${record.ttl}s`}</TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button variant="ghost" size="icon" onClick={() => openEdit(record)} title="Edit record"><Pencil className="h-4 w-4" /></Button>
                    <Button variant="ghost" size="icon" onClick={() => setConfirmRecordId(record.id)} title="Delete record"><Trash2 className="h-4 w-4 text-red-500" /></Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {(records.length === 0 || loading) && (
              <TableRow><TableCell colSpan={6} className="h-24 text-center text-slate-500">{loading ? "Loading..." : "No DNS records found."}</TableCell></TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <ConfirmDialog
        open={!!confirmRecordId}
        onOpenChange={(open) => !open && setConfirmRecordId("")}
        title="Delete DNS Record"
        description={`Delete ${recordToDelete ? `${recordToDelete.type} ${getFullRecordName(recordToDelete.name)}` : "this DNS record"}? This cannot be undone.`}
        confirmText="Delete"
        destructive
        onConfirm={() => confirmRecordId && handleDelete(confirmRecordId)}
      />
      <ConfirmDialog
        open={confirmSubdomainDelete}
        onOpenChange={setConfirmSubdomainDelete}
        title="Delete Subdomain"
        description={`Delete ${subdomain.fullDomain}? This removes the subdomain and its local DNS records from this service. This cannot be undone.`}
        confirmText="Delete Subdomain"
        destructive
        onConfirm={handleDeleteSubdomain}
      />
    </div>
  );
}

function validateRecordContent(type: DnsRecord["type"], rawContent: string) {
  const content = rawContent.trim();
  if (!content) return "Content is required.";
  if (type === "A" && !/^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/.test(content)) {
    return "A record content must be a valid IPv4 address.";
  }
  if (type === "AAAA" && !isIPv6(content)) {
    return "AAAA record content must be a valid IPv6 address.";
  }
  if ((type === "CNAME" || type === "NS") && !isDomain(content)) {
    return `${type} record content must be a valid domain name.`;
  }
  if (type === "MX") {
    const parts = content.split(/\s+/);
    const domain = parts.length === 2 && /^\d+$/.test(parts[0]) ? parts[1] : content;
    if (!isDomain(domain)) return "MX record content must be a domain name or '<priority> <domain>'.";
  }
  if (type === "TXT" && content.length > 4096) {
    return "TXT record content is too long.";
  }
  return "";
}

function canProxy(type: DnsRecord["type"]) {
  return type === "A" || type === "AAAA" || type === "CNAME";
}

function isDomain(value: string) {
  const domain = value.trim().replace(/\.$/, "").toLowerCase();
  return /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/.test(domain);
}

function isIPv6(value: string) {
  if (!/^[0-9a-fA-F:.]+$/.test(value) || !value.includes(":")) return false;
  if ((value.match(/::/g) || []).length > 1) return false;
  const [head, tail = ""] = value.split("::");
  const headParts = head ? head.split(":") : [];
  const tailParts = tail ? tail.split(":") : [];
  const parts = [...headParts, ...tailParts].filter(Boolean);
  if (parts.some((part) => part.length > 4 || !/^[0-9a-fA-F]{1,4}$/.test(part))) return false;
  return value.includes("::") ? parts.length < 8 : parts.length === 8;
}
