<script lang="ts">
	import { page } from '$app/state';
	import { openConfirmDialog } from '#lib/components/confirm-dialog/index.ts';
	import AdvancedTable from '#lib/components/table/advanced-table.svelte';
	import { Badge, type BadgeVariant } from '#lib/components/ui/badge/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as Dialog from '#lib/components/ui/dialog/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import UserService from '#lib/services/user-service.ts';
	import type {
		AdvancedTableColumn,
		CreateAdvancedTableActions
	} from '#lib/types/advanced-table.type.ts';
	import type { SignupToken } from '#lib/types/signup-token.type.ts';
	import { axiosErrorToast } from '#lib/utils/error-util.ts';
	import { Copy, Trash2 } from '@lucide/svelte';
	import { toast } from 'svelte-sonner';

	let {
		open = $bindable()
	}: {
		open: boolean;
	} = $props();

	const userService = new UserService();
	let tableRef: AdvancedTable<SignupToken>;

	function formatDate(dateStr: string | undefined) {
		if (!dateStr) return m.never();
		const date = new Date(dateStr);
		const diffMs = date.getTime() - Date.now();
		if (diffMs > 0 && diffMs < 24 * 60 * 60 * 1000) {
			const hours = Math.ceil(diffMs / (60 * 60 * 1000));
			return m.in_hours({ count: hours });
		}
		return date.toLocaleDateString();
	}

	async function revokeInvite(token: SignupToken) {
		openConfirmDialog({
			title: m.revoke_invite(),
			message: m.are_you_sure_you_want_to_revoke_this_invite(),
			confirm: {
				label: m.revoke(),
				destructive: true,
				action: async () => {
					try {
						await userService.deleteSignupToken(token.id);
						await tableRef.refresh();
						toast.success(m.invite_revoked());
					} catch (e) {
						axiosErrorToast(e);
					}
				}
			}
		});
	}

	function onOpenChange(isOpen: boolean) {
		open = isOpen;
	}

	function copyInviteLink(token: SignupToken) {
		const inviteLink = `${page.url.origin}/st/${token.token}`;
		navigator.clipboard
			.writeText(inviteLink)
			.then(() => {
				toast.success(m.copied());
			})
			.catch((err) => {
				axiosErrorToast(err);
			});
	}

	const columns: AdvancedTableColumn<SignupToken>[] = [
		{ label: m.who_is_it_for(), key: 'note', cell: NoteCell },
		{
			label: m.uses(),
			column: 'usageCount',
			sortable: true,
			cell: UsageCell
		},
		{
			label: m.expires(),
			column: 'expiresAt',
			sortable: true,
			value: (item) => formatDate(item.expiresAt)
		},
		{ label: m.approval(), column: 'requireApproval', sortable: true, cell: ApprovalCell }
	];

	const actions: CreateAdvancedTableActions<SignupToken> = () => [
		{
			label: m.copy_link(),
			icon: Copy,
			onClick: (token) => copyInviteLink(token)
		},
		{
			label: m.revoke(),
			icon: Trash2,
			variant: 'danger',
			onClick: (token) => revokeInvite(token)
		}
	];
</script>

{#snippet NoteCell({ item }: { item: SignupToken })}
	<span>{item.note || '—'}</span>
{/snippet}

{#snippet UsageCell({ item }: { item: SignupToken })}
	<div class="flex items-center gap-1 font-mono">
		{item.usageCount} / {item.usageLimit}
	</div>
{/snippet}

{#snippet ApprovalCell({ item }: { item: SignupToken })}
	<Badge class="rounded-full" variant={item.requireApproval ? 'secondary' : 'default'}>
		{item.requireApproval ? m.manual_approval() : m.automatic()}
	</Badge>
{/snippet}

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Content class="sm-min-w[500px] max-h-[90vh] min-w-[90vw] overflow-auto lg:min-w-[800px]">
		<Dialog.Header>
			<Dialog.Title>{m.manage_invites()}</Dialog.Title>
			<Dialog.Description>
				{m.view_and_manage_active_invites()}
			</Dialog.Description>
		</Dialog.Header>

		<div class="flex-1 overflow-hidden">
			<AdvancedTable
				id="invite-list"
				withoutSearch={true}
				fetchCallback={userService.listSignupTokens}
				defaultSort={{ column: 'createdAt', direction: 'asc' }}
				bind:this={tableRef}
				{columns}
				{actions}
			/>
		</div>
		<Dialog.Footer class="mt-3">
			<Button onclick={() => (open = false)}>
				{m.close()}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
