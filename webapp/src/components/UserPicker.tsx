import { useState } from 'react';
import type { UserResponse } from '@/apiclient';
import { useUsers } from '../services/users';
import { Input } from './ui/Input';

export const userLabel = (u: UserResponse) =>
    `${u.username}${u.name ? ` (${u.name})` : ''}${u.email ? ` · ${u.email}` : ''}`;

interface Props {
    id: string;
    value: string;
    onChange: (userId: string) => void;
}

export function UserPicker({ id, value, onChange }: Props) {
    const { data: users = [] } = useUsers();
    const [text, setText] = useState('');
    const selected = users.find(u => u.id === value);

    const onInput = (typed: string) => {
        setText(typed);
        onChange(users.find(u => userLabel(u) === typed)?.id ?? '');
    };

    return (
        <>
            <Input
                id={id}
                list={`${id}-users`}
                value={selected ? userLabel(selected) : text}
                onChange={e => onInput(e.target.value)}
                placeholder="Search by username, name or email"
                autoComplete="off"
            />
            <datalist id={`${id}-users`}>
                {users.map(u => (
                    <option key={u.id} value={userLabel(u)} />
                ))}
            </datalist>
        </>
    );
}
