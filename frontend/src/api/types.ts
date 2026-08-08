// Формы — ровно как отдаёт бэкенд. Не «причёсываем»: auth исторически
// отвечает snake_case, gateway — camelCase. Расхождение реальное, и типы
// обязаны его отражать, иначе оно всплывёт в рантайме.
export type UserDTO = {
  id: string;
  name: string;
  handle: string;
  bio: string;
  city: string;
  work: string;
  verified: boolean;
  avatarUrl: string | null;
  createdAt: string;
  updatedAt: string;
};

export type PostDTO = {
  id: string;
  authorId: string;
  // author === null — законное состояние: профиль автора мог не найтись
  // или user-service мог не ответить (gateway деградирует, а не падает).
  author: UserDTO | null;
  text: string;
  likes: number;
  liked: boolean;
  comments: number;
  createdAt: string;
  updatedAt: string;
};

export type FeedPage = {
  posts: PostDTO[];
  // Пустая строка означает «страниц больше нет».
  nextCursor: string;
};

export type TokenResponse = { access_token: string };

export type UpdateMePayload = Partial<
  Pick<UserDTO, 'name' | 'handle' | 'bio' | 'city' | 'work'>
>;

export type CommentDTO = {
  id: string;
  postId: string;
  authorId: string;
  // author === null — законное состояние, ровно как у PostDTO.
  author: UserDTO | null;
  text: string;
  likes: number;
  liked: boolean;
  createdAt: string;
  updatedAt: string;
};

export type CommentsPage = {
  comments: CommentDTO[];
  // Пустая строка означает «страниц больше нет».
  nextCursor: string;
};
