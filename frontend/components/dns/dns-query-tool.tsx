"use client";

import { useState } from "react";
import { Search } from "lucide-react";
import { api, DnsQueryResponse } from "@/lib/api";
import { DnsRecordType } from "@/lib/mock-data";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { toast } from "sonner";

export default function DnsQueryTool() {
  const [name, setName] = useState("");
  const [type, setType] = useState<DnsRecordType>("A");
  const [result, setResult] = useState<DnsQueryResponse | null>(null);
  const [loading, setLoading] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      setResult(await api.dnsQuery(name, type));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "DNS query failed");
      setResult(null);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      <CardHeader><CardTitle className="flex items-center gap-2"><Search className="h-5 w-5" />DNS Lookup</CardTitle></CardHeader>
      <CardContent className="space-y-4">
        <form onSubmit={submit} className="grid gap-4 md:grid-cols-[1fr_160px_auto] items-end">
          <div className="space-y-2"><Label>Domain</Label><Input value={name} onChange={(e) => setName(e.target.value)} placeholder="example.com" required /></div>
          <div className="space-y-2">
            <Label>Type</Label>
            <Select value={type} onValueChange={(value) => value && setType(value as DnsRecordType)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>{["A", "AAAA", "CNAME", "TXT", "MX", "NS"].map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          <Button type="submit" disabled={loading}>{loading ? "Querying..." : "Query"}</Button>
        </form>
        {result && (
          <div className="border rounded-lg overflow-hidden">
            <Table>
              <TableHeader><TableRow><TableHead>Name</TableHead><TableHead>TTL</TableHead><TableHead>Data</TableHead></TableRow></TableHeader>
              <TableBody>
                {(result.Answer || []).map((answer, index) => (
                  <TableRow key={`${answer.name}-${index}`}><TableCell className="font-mono">{answer.name}</TableCell><TableCell>{answer.TTL}</TableCell><TableCell className="font-mono break-all">{answer.data}</TableCell></TableRow>
                ))}
                {(!result.Answer || result.Answer.length === 0) && <TableRow><TableCell colSpan={3} className="h-20 text-center text-slate-500">No answers. Status {result.Status}</TableCell></TableRow>}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
