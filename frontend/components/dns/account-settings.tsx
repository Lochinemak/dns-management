"use client";

import { useState } from "react";
import { LockKeyhole } from "lucide-react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "sonner";

export default function AccountSettings() {
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.changePassword(oldPassword, newPassword);
      setOldPassword("");
      setNewPassword("");
      toast.success("Password updated.");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to update password");
    }
  };

  return (
    <Card>
      <CardHeader><CardTitle className="flex items-center gap-2"><LockKeyhole className="h-5 w-5" />Account Settings</CardTitle></CardHeader>
      <CardContent>
        <form onSubmit={submit} className="grid gap-4 max-w-md">
          <div className="space-y-2"><Label>Current Password</Label><Input type="password" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} required /></div>
          <div className="space-y-2"><Label>New Password</Label><Input type="password" minLength={8} value={newPassword} onChange={(e) => setNewPassword(e.target.value)} required /></div>
          <Button type="submit" className="w-fit">Update Password</Button>
        </form>
      </CardContent>
    </Card>
  );
}
