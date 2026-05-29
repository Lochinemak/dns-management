import type {Metadata} from 'next';
import './globals.css';
import { Geist } from "next/font/google";
import { cn } from "@/lib/utils";
import { LazyToaster } from "@/components/ui/lazy-toaster";

const geist = Geist({subsets:['latin'],variable:'--font-sans'});

export const metadata: Metadata = {
  title: 'Subdomain & DNS Manager',
  description: 'Manage your subdomains and DNS records.',
};

export default function RootLayout({children}: {children: React.ReactNode}) {
  return (
    <html lang="en" className={cn("font-sans", geist.variable)}>
      <body suppressHydrationWarning>
        {children}
        <LazyToaster />
      </body>
    </html>
  );
}
