import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { OrganizationDetailPage } from "./OrganizationDetailPage";

vi.mock("../services/users", () => ({
    useUsers: () => ({
        data: [
            {
                id: "user-1",
                username: "alice",
                name: "Alice",
                email: "alice@example.com",
            },
        ],
    }),
}));

vi.mock("../services/organizations", () => {
    const mutation = () => ({
        mutate: () => {},
        isPending: false,
        error: null,
    });
    return {
        useOrganization: () => ({
            data: {
                id: "org-1",
                name: "Acme Inc",
                members: [
                    { userId: "user-1", role: "owner" },
                    { userId: "user-gone", role: "member" },
                ],
            },
            isLoading: false,
            error: null,
            refetch: () => {},
        }),
        useSetOrganizationMember: mutation,
        useRemoveOrganizationMember: mutation,
        useSetOrganizationLogo: mutation,
        useDeleteOrganizationLogo: mutation,
    };
});

describe("OrganizationDetailPage", () => {
    it("shows user names in the members list, and the id when the user is unknown", () => {
        const html = renderToStaticMarkup(
            <MemoryRouter initialEntries={["/organizations/org-1"]}>
                <Routes>
                    <Route
                        path="/organizations/:id"
                        element={<OrganizationDetailPage />}
                    />
                </Routes>
            </MemoryRouter>,
        );
        expect(html).toContain("alice (Alice) · alice@example.com");
        expect(html).toContain("user-gone");
    });
});
