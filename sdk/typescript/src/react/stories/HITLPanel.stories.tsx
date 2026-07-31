/**
 * HITLPanel.stories.tsx — HITLPanel 组件 Storybook stories。
 */

import type { Meta, StoryObj } from '@storybook/react';
import { HITLPanel } from '../collaboration/HITLPanel.js';

const meta = {
  title: 'Collaboration/HITLPanel',
  component: HITLPanel,
  tags: ['autodocs'],
} satisfies Meta<typeof HITLPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {
  args: { approvals: [] },
};

export const WithApprovals: Story = {
  args: {
    approvals: [
      { id: 'ap1', agentId: 'a1', title: 'Deploy to production', detail: 'Release v2.0.0' },
      { id: 'ap2', agentId: 'a2', title: 'Execute database migration', detail: 'ALTER TABLE users ADD COLUMN role TEXT' },
      { id: 'ap3', agentId: 'a1', title: 'Send notification email' },
    ],
    onApprove: (id: string) => console.log('Approved:', id),
    onReject: (id: string) => console.log('Rejected:', id),
  },
};

export const SingleApproval: Story = {
  args: {
    approvals: [
      { id: 'ap1', agentId: 'a1', title: 'Confirm action', detail: 'This action cannot be undone.' },
    ],
  },
};
