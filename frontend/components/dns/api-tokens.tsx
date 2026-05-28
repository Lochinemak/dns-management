"use client";

import { useEffect, useState } from "react";
import { Check, Copy, KeyRound, Plus, RotateCw } from "lucide-react";
import { api } from "@/lib/api";
import { ApiToken } from "@/lib/mock-data";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { toast } from "sonner";

export default function ApiTokens() {
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [isAddOpen, setIsAddOpen] = useState(false);
  const [newTokenName, setNewTokenName] = useState("");
  const [generatedToken, setGeneratedToken] = useState<string | null>(null);
  const [generatedTokenName, setGeneratedTokenName] = useState("");
  const [copied, setCopied] = useState(false);
  const [busyTokenId, setBusyTokenId] = useState("");
  const [error, setError] = useState("");
  const [confirmAction, setConfirmAction] = useState<{ type: "rotate" | "revoke"; id: string } | null>(null);

  const load = async () => {
    try {
      const nextTokens = await api.tokens();
      setTokens(Array.isArray(nextTokens) ? nextTokens : []);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to load tokens");
    }
  };

  useEffect(() => {
    load();
  }, []);

  const handleGenerate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setError("");
      const created = await api.createToken(newTokenName.trim());
      setTokens([created, ...tokens]);
      setGeneratedToken(created.token || null);
      setGeneratedTokenName(created.name);
      setNewTokenName("");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to create token";
      setError(message);
      toast.error(message);
    }
  };

  const handleCopy = async () => {
    if (!generatedToken) return;
    await navigator.clipboard.writeText(generatedToken);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
    toast.success("Token copied");
  };

  const handleRevoke = async (id: string) => {
    try {
      setError("");
      await api.deleteToken(id);
      setTokens(tokens.filter((token) => token.id !== id));
      setConfirmAction(null);
      toast.success("Token revoked.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to revoke token";
      setError(message);
      toast.error(message);
    }
  };

  const handleRotate = async (id: string) => {
    setBusyTokenId(id);
    setError("");
    try {
      const rotated = await api.rotateToken(id);
      setTokens(tokens.map((token) => token.id === id ? { ...token, createdAt: rotated.createdAt } : token));
      setGeneratedToken(rotated.token || null);
      setGeneratedTokenName(rotated.name);
      setConfirmAction(null);
      setIsAddOpen(true);
      toast.success("Token rotated. The previous token is now invalid.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to rotate token";
      setError(message);
      toast.error(message);
    } finally {
      setBusyTokenId("");
    }
  };
  const tokenToConfirm = tokens.find((token) => token.id === confirmAction?.id);

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">API Tokens</h2>
          <p className="text-slate-500">Manage programmatic access tokens.</p>
        </div>
        <Dialog open={isAddOpen} onOpenChange={(open) => { setIsAddOpen(open); if (!open) setGeneratedToken(null); }}>
          <Button className="gap-2" onClick={() => setIsAddOpen(true)}><Plus className="h-4 w-4" />Generate Token</Button>
          <DialogContent>
            {generatedToken ? (
              <>
                <DialogHeader><DialogTitle>Token Generated</DialogTitle></DialogHeader>
                <Alert>
                  <AlertTitle>Save this token now</AlertTitle>
                  <AlertDescription>
                    {generatedTokenName ? `${generatedTokenName}: ` : ""}This token is shown only once. Store it securely before closing this dialog.
                  </AlertDescription>
                </Alert>
                <div className="flex items-center gap-2 pt-4">
                  <Input readOnly value={generatedToken} className="font-mono" />
                  <Button variant="outline" size="icon" onClick={handleCopy}>{copied ? <Check className="h-4 w-4 text-emerald-500" /> : <Copy className="h-4 w-4" />}</Button>
                </div>
                <DialogFooter><Button onClick={() => setIsAddOpen(false)}>Done</Button></DialogFooter>
              </>
            ) : (
              <form onSubmit={handleGenerate} className="space-y-4">
                <DialogHeader><DialogTitle>Generate API Token</DialogTitle></DialogHeader>
                <div className="space-y-2">
                  <Label>Token Name</Label>
                  <Input placeholder="CI/CD" value={newTokenName} onChange={(e) => setNewTokenName(e.target.value)} required />
                </div>
                <DialogFooter><Button type="submit">Generate</Button></DialogFooter>
              </form>
            )}
          </DialogContent>
        </Dialog>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertTitle>API token operation failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="border rounded-lg bg-white dark:bg-slate-900 shadow-sm overflow-hidden">
        <Table>
          <TableHeader><TableRow><TableHead>Token Name</TableHead><TableHead>Created At</TableHead><TableHead className="text-right">Action</TableHead></TableRow></TableHeader>
          <TableBody>
            {tokens.map((token) => (
              <TableRow key={token.id}>
                <TableCell><div className="flex items-center gap-2"><KeyRound className="h-4 w-4 text-slate-400" />{token.name}</div></TableCell>
                <TableCell className="text-slate-500 text-sm">{new Date(token.createdAt).toLocaleString()}</TableCell>
                <TableCell className="text-right space-x-2">
                  <Button variant="outline" size="sm" onClick={() => setConfirmAction({ type: "rotate", id: token.id })} disabled={busyTokenId === token.id}>
                    <RotateCw className="h-4 w-4 mr-1" />
                    {busyTokenId === token.id ? "Rotating..." : "Rotate"}
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setConfirmAction({ type: "revoke", id: token.id })} className="text-red-500">Revoke</Button>
                </TableCell>
              </TableRow>
            ))}
            {tokens.length === 0 && <TableRow><TableCell colSpan={3} className="h-24 text-center text-slate-500">No API tokens generated yet.</TableCell></TableRow>}
          </TableBody>
        </Table>
      </div>
      <ConfirmDialog
        open={!!confirmAction}
        onOpenChange={(open) => !open && setConfirmAction(null)}
        title={confirmAction?.type === "rotate" ? "Rotate API Token" : "Revoke API Token"}
        description={
          confirmAction?.type === "rotate"
            ? `Rotate ${tokenToConfirm?.name || "this API token"}? The current token value will stop working immediately.`
            : `Revoke ${tokenToConfirm?.name || "this API token"}? This cannot be undone.`
        }
        confirmText={confirmAction?.type === "rotate" ? "Rotate" : "Revoke"}
        destructive={confirmAction?.type === "revoke"}
        loading={!!confirmAction && busyTokenId === confirmAction.id}
        onConfirm={() => {
          if (!confirmAction) return;
          if (confirmAction.type === "rotate") handleRotate(confirmAction.id);
          if (confirmAction.type === "revoke") handleRevoke(confirmAction.id);
        }}
      />
    </div>
  );
}
