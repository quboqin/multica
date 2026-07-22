import type { Metadata } from "next";
import { MulticaLanding } from "@/features/landing/components/multica-landing";
import { RedirectIfAuthenticated } from "@/features/landing/components/redirect-if-authenticated";

export const metadata: Metadata = {
  title: {
    absolute: "Cybertron - Project Management for Human + Agent Teams",
  },
  description:
    "Cybertron is an open-source platform that turns coding agents into real teammates. Assign tasks, track progress, compound skills.",
  openGraph: {
    title: "Cybertron - Project Management for Human + Agent Teams",
    description:
      "Manage your human + agent workforce in one place with Cybertron.",
    url: "/",
  },
  alternates: {
    canonical: "/",
  },
};

export default function LandingPage() {
  return (
    <>
      <RedirectIfAuthenticated />
      <MulticaLanding />
    </>
  );
}
