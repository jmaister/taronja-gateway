import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { useCreateUser } from '../services/services';
import { UserCreateRequest } from '@/apiclient';
import { Button } from '../components/ui/Button';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { FormField } from '../components/ui/FormField';
import { Input } from '../components/ui/Input';
import { PageHeader } from '../components/ui/PageHeader';

export function CreateUserPage() {
    const navigate = useNavigate();
    const [formData, setFormData] = useState<UserCreateRequest>({
        username: '',
        email: '',
        password: '',
    });
    const [message, setMessage] = useState('');
    const [messageType, setMessageType] = useState<'success' | 'error' | ''>('');
    const createUserMutation = useCreateUser();

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const { name, value } = e.target;
        setFormData((prevFormData: UserCreateRequest) => ({
            ...prevFormData,
            [name]: value,
        }));
    };

    const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        setMessage('');
        setMessageType('');
        createUserMutation.mutate(formData, {
            onSuccess: (createdUser) => {
                if (createdUser) {
                    setMessage(
                        `User created successfully! ID: ${createdUser.id}, Username: ${createdUser.username}. Redirecting...`
                    );
                    setMessageType('success');
                    setFormData({ username: '', email: '', password: '' });
                    setTimeout(() => {
                        navigate(`/users/${createdUser.id}`);
                    }, 1500);
                }
            },
            onError: (err: any) => {
                setMessage(err.message || 'An unexpected error occurred. Please try again.');
                setMessageType('error');
            },
        });
    };

    let messageClasses = 'mb-5 rounded-md border p-3 text-sm';
    if (messageType === 'success') {
        messageClasses += ' border-success/20 bg-success/10 text-success';
    } else if (messageType === 'error') {
        messageClasses += ' border-danger/20 bg-danger/10 text-danger';
    }

    return (
        <div className="mx-auto max-w-2xl space-y-6">
            <PageHeader title="User Management" description="Create a new user account" />

            <Card>
                <CardHeader>
                    <h2 className="text-base font-semibold">Create New User</h2>
                </CardHeader>
                <CardContent>
                    {message && messageType && (
                        <div className={messageClasses} role="alert">
                            {message}
                        </div>
                    )}
                    <form onSubmit={handleSubmit} className="space-y-5">
                        <FormField label="Username" htmlFor="username" required>
                            <Input
                                type="text"
                                id="username"
                                name="username"
                                value={formData.username}
                                onChange={handleChange}
                                required
                                disabled={createUserMutation.isPending}
                            />
                        </FormField>
                        <FormField label="Email" htmlFor="email" required>
                            <Input
                                type="email"
                                id="email"
                                name="email"
                                value={formData.email}
                                onChange={handleChange}
                                required
                                disabled={createUserMutation.isPending}
                            />
                        </FormField>
                        <FormField label="Password" htmlFor="password" required>
                            <Input
                                type="password"
                                id="password"
                                name="password"
                                value={formData.password}
                                onChange={handleChange}
                                required
                                disabled={createUserMutation.isPending}
                            />
                        </FormField>
                        <Button type="submit" disabled={createUserMutation.isPending} className="w-full">
                            {createUserMutation.isPending ? 'Creating…' : 'Create User'}
                        </Button>
                    </form>
                    <div className="mt-8 border-t border-border pt-6 text-center text-sm">
                        <Link to="/" className="tg-link mr-5">
                            &larr; Home
                        </Link>
                        <Link to="/users" className="tg-link">
                            View All Users
                        </Link>
                    </div>
                </CardContent>
            </Card>
        </div>
    );
}
