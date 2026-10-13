-- =============================================================================
-- 小说板块 migration：书目 / 章节 / 评论（书评·章评·段评）/ 书架 / 阅读进度
-- 目标库: ssemarket
-- 说明:
--   1. 全部为新增表，不改动既有表；建表已由 common/database.go 的 GORM AutoMigrate 自动完成，
--      本文件仅作 schema 参照与演示种子数据（按需手动执行种子段）。
--   2. 种子数据（novel_id 9001~9003）为演示内容，可整段删除，回滚见文末。
--   3. 幂等：建表用 IF NOT EXISTS，种子用 INSERT IGNORE + 显式主键，可重复执行。
-- =============================================================================

USE ssemarket;

-- ---------------------------------------------------------------------------
-- 1) 书目
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `novels` (
  `novel_id` INT NOT NULL AUTO_INCREMENT,
  `user_id` INT NOT NULL COMMENT '作者 userID',
  `title` VARCHAR(100) NOT NULL,
  `author_name` VARCHAR(50) NOT NULL DEFAULT '' COMMENT '展示用笔名，空则回退用户昵称',
  `cover_url` VARCHAR(512) NOT NULL DEFAULT '',
  `category` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '校园/悬疑/科幻…',
  `intro` VARCHAR(1000) NOT NULL DEFAULT '',
  `status` VARCHAR(16) NOT NULL DEFAULT 'serializing' COMMENT 'serializing|completed',
  `chapter_num` INT NOT NULL DEFAULT 0,
  `word_num` INT NOT NULL DEFAULT 0,
  `read_num` INT NOT NULL DEFAULT 0,
  `shelf_num` INT NOT NULL DEFAULT 0 COMMENT '加入书架人数',
  `comment_num` INT NOT NULL DEFAULT 0 COMMENT '书评+章评+段评总数',
  `latest_chapter_id` INT NOT NULL DEFAULT 0,
  `latest_chapter_title` VARCHAR(200) NOT NULL DEFAULT '',
  `latest_chapter_at` DATETIME NULL,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`novel_id`),
  KEY `idx_novel_user` (`user_id`),
  KEY `idx_novel_category` (`category`),
  KEY `idx_novel_status` (`status`),
  KEY `idx_novel_latest` (`latest_chapter_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='小说书目';

-- ---------------------------------------------------------------------------
-- 2) 章节（正文段落以换行分隔，段评序号 = 去空白后的段落下标）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `novel_chapters` (
  `chapter_id` INT NOT NULL AUTO_INCREMENT,
  `novel_id` INT NOT NULL,
  `chapter_num` INT NOT NULL COMMENT '本书内序号，从 1 递增，用于上下章导航',
  `title` VARCHAR(200) NOT NULL,
  `content` LONGTEXT NOT NULL,
  `author_note` VARCHAR(2000) NOT NULL DEFAULT '' COMMENT '作者有话说',
  `word_num` INT NOT NULL DEFAULT 0,
  `comment_num` INT NOT NULL DEFAULT 0 COMMENT '本章章评+段评数',
  `read_num` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`chapter_id`),
  UNIQUE KEY `uk_chapter_novel_num` (`novel_id`, `chapter_num`),
  KEY `idx_chapter_novel_num` (`novel_id`, `chapter_num`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='小说章节';

-- ---------------------------------------------------------------------------
-- 3) 评论：书评(book) / 章评(chapter) / 段评(paragraph) 同表
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `novel_comments` (
  `comment_id` INT NOT NULL AUTO_INCREMENT,
  `novel_id` INT NOT NULL,
  `chapter_id` INT NOT NULL DEFAULT 0 COMMENT '书评固定为 0',
  `paragraph_index` INT NOT NULL DEFAULT -1 COMMENT '段评段落下标；非段评为 -1',
  `target_type` VARCHAR(16) NOT NULL DEFAULT 'book' COMMENT 'book|chapter|paragraph',
  `paragraph_excerpt` VARCHAR(120) NOT NULL DEFAULT '' COMMENT '被评论段落摘要，评论面板展示',
  `user_id` INT NOT NULL,
  `content` VARCHAR(500) NOT NULL,
  `like_num` INT NOT NULL DEFAULT 0,
  `deny_num` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`comment_id`),
  KEY `idx_novel_comment_novel` (`novel_id`, `target_type`),
  KEY `idx_novel_comment_paragraph` (`chapter_id`, `target_type`, `paragraph_index`),
  KEY `idx_novel_comment_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='小说评论(书评/章评/段评)';

CREATE TABLE IF NOT EXISTS `novel_comment_likes` (
  `like_id` INT NOT NULL AUTO_INCREMENT,
  `comment_id` INT NOT NULL,
  `user_id` INT NOT NULL,
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`like_id`),
  UNIQUE KEY `uk_novel_comment_like` (`comment_id`, `user_id`),
  KEY `idx_novel_comment_like` (`comment_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='小说评论点赞';

-- ---------------------------------------------------------------------------
-- 4) 书架与阅读进度
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `novel_bookshelf` (
  `shelf_id` INT NOT NULL AUTO_INCREMENT,
  `novel_id` INT NOT NULL,
  `user_id` INT NOT NULL,
  `created_at` DATETIME NOT NULL COMMENT '加入时间',
  PRIMARY KEY (`shelf_id`),
  UNIQUE KEY `uk_novel_shelf` (`novel_id`, `user_id`),
  KEY `idx_novel_shelf_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='小说书架';

CREATE TABLE IF NOT EXISTS `novel_progress` (
  `progress_id` INT NOT NULL AUTO_INCREMENT,
  `novel_id` INT NOT NULL,
  `user_id` INT NOT NULL,
  `chapter_id` INT NOT NULL DEFAULT 0,
  `chapter_num` INT NOT NULL DEFAULT 0,
  `paragraph_index` INT NOT NULL DEFAULT 0,
  `percent` DOUBLE NOT NULL DEFAULT 0 COMMENT '全书进度 0~100',
  `updated_at` DATETIME NOT NULL,
  PRIMARY KEY (`progress_id`),
  UNIQUE KEY `uk_novel_progress` (`novel_id`, `user_id`),
  KEY `idx_novel_progress_user` (`user_id`, `updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='小说阅读进度';

-- =============================================================================
-- 种子数据（演示用，可删除；删除方式见文末回滚）
-- =============================================================================

INSERT IGNORE INTO `novels`
  (`novel_id`, `user_id`, `title`, `author_name`, `cover_url`, `category`, `intro`, `status`, `created_at`, `updated_at`)
VALUES
  (9001, 278, '晚自习后的银河', '秋', '', '校园',
   '停电的十一分钟里，全班把银河记了一遍。一本蓝色格子的本子，从高二传到毕业，每一页都有人加了一句话。',
   'serializing', NOW(), NOW()),
  (9002, 278, '第七层楼梯', '秋', '', '悬疑',
   '宿舍楼只有六层，可所有人都记得第七层。宿管说那是幻觉，直到有人在第七层丢了一把钥匙。',
   'serializing', NOW(), NOW()),
  (9003, 278, '深空邮路', '秋', '', '科幻',
   '太阳系邮政第七号线，一趟跑十一年。邮差老陈送了一辈子别人的信，最后一封是寄给自己的。',
   'completed', NOW(), NOW());

-- ---- 9001 晚自习后的银河 ----
INSERT IGNORE INTO `novel_chapters`
  (`chapter_id`, `novel_id`, `chapter_num`, `title`, `content`, `author_note`, `created_at`, `updated_at`)
VALUES
  (91001, 9001, 1, '第一章 停电的晚自习',
   '晚自习进行到第三十分钟，灯灭了。\n整栋教学楼像被人按下了暂停键，只有走廊尽头的安全出口灯还亮着，把一片绿光投在物理老师的镜片上。\n别慌。他说，把笔放下，让眼睛适应黑暗。\n我坐在靠窗的位置，听见窗外梧桐叶被风翻动的声音。那一刻我忽然觉得，黑暗比白天更安静。\n前排的男生开始小声背元素周期表，背到钾钙钠镁铝就卡住了，全班笑成一片。\n我也笑了。笑完才发现，自己已经很久没有这样笑过。\n停电持续了十一分钟。后来我数过，因为那十一分钟里我把银河记了一遍。\n不是课本上那张图，是我小时候在外婆家院子里看到的那条。\n外婆说，人这一辈子只要看清一次银河，就不怕黑了。\n灯亮起来的时候，黑板上的力学图还停在半空，粉笔灰落了一桌。\n物理老师拍拍手上的灰，说刚才那十一分钟算你们休息。\n我把台灯拧到最亮，忽然很想把今晚写下来。\n于是我买了这个本子，蓝色格子，第一页写着：停电夜，全班都在笑。\n后来这本子被传了很多次，每一页都有人加了一句话。\n再后来，它成了我们班最旧的一本小说。',
   '这一章来自高二那年一次真实的停电。你们学校有过停电吗？评论区告诉我那晚你在做什么。',
   NOW(), NOW()),
  (91002, 9001, 2, '第二章 同桌的第三张草稿纸',
   '同桌的草稿纸永远只有三张，用完就换新的，旧的撕成一条一条塞进笔袋。\n她说这样记得清楚：一张写错的，一张写对的，一张留给还没想明白的。\n我第一次见到这种规矩是在九月的最后一节数学课。\n那天讲圆锥曲线，她把椭圆画得比课本还圆，然后在第三张纸上写：这题我还没懂。\n我以为第三张纸会一直空着。结果第二天它被写满了，字迹比前两张都用力。\n第三个月，她的第三张纸开始借给我。\n我写错的解法，她会在旁边画一个很小的箭头，箭头指向她自己的解法，从不写你错了。\n期末前一周，她把三张纸全空着，只写了一行：这周我想自己走完。\n后来我才知道，那周她每天晚自习后在走廊背单词，背到保安关灯。\n毕业那天她把笔袋里的纸条全倒出来，说这些是她一学期的错误集合。\n我捡了一张，上面写着：椭圆是圆被拉长以后的样子，不是圆坏了。',
   '写这一章的时候我把高中草稿纸翻出来看过。原来我所有的错误都被自己扔掉了。',
   NOW(), NOW()),
  (91003, 9001, 3, '第三章 走廊尽头的天文社',
   '天文社的活动室在走廊尽头，门牌是自己打印的，A4 纸边角已经卷起来。\n社长只有一个人，就是社长本人，他坚持不招新，理由是招了新就要负责。\n他有一台旧望远镜，镜筒上贴着一圈医用胶带，是他自己缠的，说是为了遮光。\n第一次观测安排在十一月的一个周五，地点是操场看台最高一层。\n那天风很大，他把镜头盖摘下来又盖上，反复四次，最后说今晚不行。\n我们站在看台上等了四十分钟，谁都没走。\n后来他妥协了，把望远镜架在栏杆上，说只看月亮，月亮不怕风。\n我看的第一眼是月亮的边缘，黑得很干净，像刀切出来的。\n第二眼我什么也没看见，只看见目镜里一片白，是我自己的呼吸。\n社长说，看天文第一件事是学会等，等眼睛稳，等空气稳，等自己不那么急。\n那天回去的路上，走廊的灯坏了一半，剩下的那半亮得很难看。\n可我们走得很慢，因为谁都想再看一下月亮有没有被风吹跑。',
   '这一章写一个真实存在过的人。他不让我写他的名字，所以书里他只有社长两个字。',
   NOW(), NOW()),
  (91004, 9001, 4, '第四章 蓝色本子的第一次争吵',
   '本子传到第五个人的时候，被撕掉了一页。\n撕的人是我们班最不爱说话的那个，他撕完就把纸团扔进垃圾桶，谁也没敢问。\n被撕掉的那页写着一句很普通的话：今天食堂的汤很好喝。\n第二天他在本子第一页加了一行：我把它撕了，因为那句话是我写的，我写的时候手在抖。\n全班安静了一整天。\n放学后社长把本子拿去天文社，说放在这里最安全，因为没人去。\n结果那个月天文社来了一个新成员，是来问望远镜能不能看土星环的。\n新成员翻到本子，读了一半，问这是谁写的。\n社长说，谁都可以写，但只能加一句。\n新成员加了一句：土星环其实很薄，薄得像纸。\n那天晚上本子被风吹到操场，第二天在球门底下找到，封面全是泥。\n我们把它擦干净，谁都没提撕页的事。\n有些东西旧了以后才看得出它到底被多少人小心地传过。',
   '写到第四章我停下来想过：如果这本书只有我一个人写，它还会被小心传递吗。',
   NOW(), NOW()),
  (91005, 9001, 5, '第五章 毕业前的最后一次晚自习',
   '最后一次晚自习没有考试，班主任说你们想干什么就干什么。\n有人把课桌拼成一张长桌，上面铺满同学录，签名从第一页排到最后一页。\n有人站在讲台上背元素周期表，背到钾钙钠镁铝还是卡住了，全班笑成一片。\n我发现笑是有回声的，三年前的回声和今天的回声撞在一起，几乎分不出先后。\n同桌把三张草稿纸摊在桌上，第三张是空的。\n她说这张留给你，你写一句。\n我写了：停电那晚我记了一遍银河，现在还记得。\n她看完在自己的那张上写：那晚我也在记，我记的是你笑了。\n十点半熄灯，走廊的灯比教室先灭。\n我们站在走廊上，谁都没回教室。\n物理老师从办公室出来，看见我们，说了一句：眼睛适应一下黑暗。\n然后他回去了，门在他身后合上，很轻。\n那天本子被传到最后一个人，最后一页只剩半行空白。\n半行也够了。够写一句：我们毕业了。',
   '这一章写完我把它念给同桌听过。她说最后一句太短。我说短的那句才是真的。',
   NOW(), NOW()),
  (91006, 9001, 6, '第六章 银河不需要记住',
   '多年以后我回学校，教学楼翻新了，走廊尽头的安全出口灯换成了 LED，白得刺眼。\n天文社的活动室变成储物间，门牌还挂着，纸已经黄透。\n宿管认不出我，我报出班级，他想了很久，说那个班出过一届挺安静的学生。\n我在操场看台最高一层站了一会儿，风还是很大。\n月亮没出来。\n我打开手机想查那晚停电的日期，查到了，是十一月十七号。\n十一月十七号，全班在黑暗里记了一遍银河。\n其实银河不需要记住，需要记住的是那十一分钟里没有人害怕。\n本子我带来了，最后一页那半行空白被人补上了，字迹不是我认识的任何人。\n补的是：后来我们都成了会给别人画箭头的人。\n我把本子留在看台上，压在栏杆下面。\n如果哪天又停电，希望有人找到它，再加一句。\n黑暗不是用来怕的，是用来等的。',
   '这本书到这里先停一下。后面还有三章在写，关于那个撕页的人。想看的同学在评论区说一声。',
   NOW(), NOW());

-- ---- 9002 第七层楼梯 ----
INSERT IGNORE INTO `novel_chapters`
  (`chapter_id`, `novel_id`, `chapter_num`, `title`, `content`, `author_note`, `created_at`, `updated_at`)
VALUES
  (92001, 9002, 1, '第一章 六层楼与七层人',
   '宿舍楼只有六层，图纸上写着六层，验收报告上写着六层。\n可住过这栋楼的人都记得第七层，说得最肯定的是三楼东侧的那几个。\n他们说第七层的楼梯比下面的窄，扶手是铁的，冬天冰手。\n宿管老周听了二十多年，回答永远是同一句：那是幻觉，人在夜里容易多出一层。\n直到有人真的在第七层丢了一把钥匙。\n丢钥匙的人叫许蔓，她上夜班，回来时已经过了十二点。\n她说她上到第七层才发现自己没带房卡，于是回六层拿备用钥匙，再上去时楼梯只到六层。\n她把这件事写在报修单上，老周在单子上批了四个字：楼层无误。\n真正让事情变复杂的是钥匙本身。\n那把钥匙是实验室的，齿形很怪，全楼只有三个人有同款。\n另外两个人都说自己那把还在。\n第三个人没有说话，因为他那天晚上也在楼上，但他不肯说自己在第几层。',
   '写悬疑最难的是让每一句都同时是线索和废话。这一章我改了七遍。',
   NOW(), NOW()),
  (92002, 9002, 2, '第二章 报修单上的第四个字',
   '报修单是复写的，一式三联，白色留楼里，粉色给后勤，蓝色给报修人。\n许蔓那张的粉色联在后勤档案柜里躺了三个月，没人处理。\n后勤的人说，单子上写楼层无误，那就是没问题。\n三个月后档案柜搬家，单子被翻出来，因为另一张单子上写着同一栋楼的第七层。\n第二张单的报修人是老周本人，内容是：七层灯坏，请换。\n老周不承认自己写过。\n他签字栏的字是他的，笔迹鉴定也说是他的。\n但那天他在医院陪床，有住院记录。\n后勤把两张单并在一起看，发现批语的字和签字的字不是同一个人写的。\n批语是楼层无误，笔画很硬，像用尺子比着写。\n签字是老周的，软，末尾往下拖。\n也就是说，有人替老周批了单，也有人替老周签了单。\n而这两件事都发生在夜里。',
   '',
   NOW(), NOW()),
  (92003, 9002, 3, '第三章 铁扶手上的温度',
   '许蔓开始数楼梯。\n她每天上夜班前数一次，回来再数一次，数到第几步停，第几步开始变窄。\n她数了十九天，前十八天都是六层。\n第十九天她数到七层，而且写进了手机备忘录。\n那天她做了两件事：在第七层的扶手上摸了一下，在第六层的扶手上也摸了一下。\n她说第六层的扶手是凉的，第七层的扶手是温的。\n铁不会自己发热，除非有人刚握过。\n第二天她在同一时间上去，扶手是凉的。\n第三天又是温的。\n她把时间排出来，温的那几次都落在凌晨两点到四点之间。\n那段时间楼里只有四个人醒着：她，一个失眠的考研生，一个每天凌晨出去跑步的男生，还有老周。\n跑步的男生说自己从不走楼梯，他走消防通道。\n考研生说自己那几天根本不在学校。\n老周说，我值夜班，但我从不查七层，因为七层没有门。',
   '这一章的推理链我留了一个明显的洞。看出来的人请在段评里指出来，别在书评里剧透。',
   NOW(), NOW()),
  (92004, 9002, 4, '第四章 没有门的楼层',
   '没有门的楼层是怎么存在的。\n老周的解释是：七层是一段楼梯加一个平台，平台通向设备间，设备间常年锁着。\n也就是说七层不是房间，是一段路。\n一段路不需要门，只需要有人走。\n许蔓把钥匙的事重新讲了一遍。她强调自己没带房卡，说明她当时确信自己住在六层。\n她说这句话的时候停了一下，然后补了一句：我那时也确信自己在往上走。\n问题不在楼层，在她当时的确信。\n老周听完这句话以后，从抽屉里拿出一本旧值班记录。\n记录里有一行，日期是钥匙丢失那晚：两点四十，七层有光。\n老周说他当时以为是设备间的灯没关。\n他去查了设备间的灯，设备间的灯是坏的，坏了两年。\n那么两点四十在七层亮着的，不是灯。\n许蔓问：那是什么。\n老周把值班记录合上，说：是一个不肯说自己在第几层的人。',
   '',
   NOW(), NOW()),
  (92005, 9002, 5, '第五章 第三把钥匙',
   '第三把钥匙的主人叫程默，就是那个不肯说自己在第几层的人。\n他肯说的部分是这样的：他每晚两点从消防通道上楼，因为他打不开正门的门禁。\n他肯说的部分还包括：他从不走七层，因为他怕高。\n怕高的人不会怕六层，六层只有三米多。\n老周问他在怕什么。\n他说他在怕楼梯变窄。\n他说楼梯一变窄他就知道自己在往上走，可他一往上走就分不清自己在往哪儿走。\n许蔓把这句话记下来，记完她翻出自己那十九天的计数。\n前十八天，楼梯在第几步变窄，她写得清清楚楚。\n第十八天变窄的位置比前十七天早了两步。\n第十九天早了四步。\n也就是说，变窄的位置一直在往楼下移。\n它不是第七层在出现，是第六层在变窄。\n程默每晚都在把某一段楼梯变窄一点。\n他手里那把齿形很怪的钥匙，开的是设备间的锁。\n设备间在七层平台上，里面有一台老式除湿机，常年开着。\n除湿机的外壳是热的。铁扶手变温，是因为有人每晚握着它站在那台机器旁边。',
   '真相到这里只到一半。剩下的一半关于他为什么每晚都要站在那里。这一章我先写到这儿。',
   NOW(), NOW());

-- ---- 9003 深空邮路 ----
INSERT IGNORE INTO `novel_chapters`
  (`chapter_id`, `novel_id`, `chapter_num`, `title`, `content`, `author_note`, `created_at`, `updated_at`)
VALUES
  (93001, 9003, 1, '第一章 第七号线',
   '太阳系邮政第七号线从地球出发，经火星轨道补给站，到木卫二，单程十一年零四个月。\n这条线不载人，只载信。\n老陈在这条线上跑了三十七年，跑完十二个来回。\n他记得每一个站点的重力差：火星补给站轻得让人想跳，木卫二重得让人想坐。\n第七号线有一台老式分拣机，纸信进去，按目的地吐出来，卡纸的时候声音像咳嗽。\n年轻邮差用扫描阵列，三秒读完一船货。\n老陈还是用手分拣，他说手能摸出信的重量。\n扫描阵列读不出重量。\n重量不是克数，是一个人写这封信花了多久。\n有一封从土星环监测站寄出的信，只有八个字：妈，我这边很好，别担心。\n老陈掂了一下，说这封写了三年。\n扫描阵列显示：正文八字，附件无。\n它说得也对。它只是不知道那八个字里有多少次删掉重写。',
   '这一章的分拣机是我从一台老式打印机想象出来的。你们还有会咳嗽的机器吗。',
   NOW(), NOW()),
  (93002, 9003, 2, '第二章 十一年零四个月的回信',
   '第七号线最贵的不是燃料，是等待。\n一个监测站的人寄出一封信，回信要等二十二年。\n二十二年足够一个孩子长大，足够一段婚姻结束，足够一个人把名字换掉。\n邮政有一条规矩：收件人姓名变更超过二十年的，信件退回出发站。\n退回的信件堆在地球站的三号库房，堆到天花板。\n老陈每年清一次，清的时候他会把每封读一遍。\n他知道这些信里写了什么，因为他读过太多次同样的开头。\n开头通常是：我看到你的信了，我这边一切都好。\n老陈说，人在等很久以后，会先报平安，再讲别的。\n因为怕对方担心，也怕自己失望。\n三号库房有一封没有开头的信，第一句是：我不记得你长什么样了。\n老陈把这封读了三遍。\n第三遍读完他把它放回箱子，没有退回，也没有投递。\n他在投递单上写：待确认收件人。这一栏他写了三十七年，只有这一封是给自己的。',
   '',
   NOW(), NOW()),
  (93003, 9003, 3, '第三章 木卫二的冰与不结冰',
   '木卫二补给站在冰层下面十一公里，靠地热活着。\n补给站的人一年见一次太阳，见的时候全站放假一天。\n放假那天所有人挤在观察舱里，看探测器传回来的画面。\n画面里的太阳比星星亮不了多少。\n有个新来的研究员问，既然太阳这么小，为什么还要看。\n站长说，因为它是我们的太阳，小也得看。\n老陈那年第五次到补给站，他带了四百封信，其中六十一封是同一家人寄的。\n六十一封，十一年里每月一封，署名都是同一个女孩。\n女孩从九岁写到二十岁。\n老陈把六十一封按顺序交出去，收件人是她父亲。\n父亲读第一封的时候十九岁，读最后一封的时候三十岁。\n他说了一句让老陈记了很多年的话：我女儿在这十一年里长完了。\n她说的是信，不是她自己。\n冰层下面十一公里，一个人靠十一年的信把女儿养大。',
   '这一章的数据我算过：每月一封、十一年，正好六十一封。写科幻也得算数。',
   NOW(), NOW()),
  (93004, 9003, 4, '第四章 最后一封',
   '老陈退休前最后一次跑第七号线，出发前他把那封待确认收件人的信从库房取了出来。\n信没有署名，收件人栏写着：给还在跑船的人。\n他在库房里放了三十年。\n老陈把它拆开，里面只有一张航线图，第七号线，每一站都被铅笔圈过。\n圈的力度不一样，有的很轻，有的把纸划破了。\n划破的是木卫二。\n图的背面写着一行小字：第七号线不载人，但有人。\n老陈把这张图带回船上，钉在分拣机旁边。\n年轻邮差问这是谁写的。\n老陈想了很久，说写的人一定在这条线上待过很久，久到他知道每一站的重量。\n回到地球站那天，老陈把投递单上待确认收件人那一栏改成：已确认。\n他把信放回三号库房，放在最上面。\n他没有把它寄出去，因为第七号线还在跑，只要还在跑，这封信就一直有人收到。\n后来第七号线换了新的分拣机，不咳嗽了。\n老陈退休那天在航线上写了一句话，只有七个字：手能摸出重量。',
   '完结。这一章我写得最快，因为结尾我想了很多年，早就写好了。',
   NOW(), NOW());

-- ---- 统计字段回填（种子数据不手写字数，交给 SQL 计算） ----
UPDATE `novel_chapters`
SET `word_num` = CHAR_LENGTH(REPLACE(REPLACE(REPLACE(`content`, '\n', ''), ' ', ''), '\t', ''))
WHERE `novel_id` IN (9001, 9002, 9003);

UPDATE `novels` n
SET n.`chapter_num` = (SELECT COUNT(*) FROM `novel_chapters` c WHERE c.`novel_id` = n.`novel_id`),
    n.`word_num`    = (SELECT COALESCE(SUM(c.`word_num`), 0) FROM `novel_chapters` c WHERE c.`novel_id` = n.`novel_id`)
WHERE n.`novel_id` IN (9001, 9002, 9003);

UPDATE `novels` n
SET n.`latest_chapter_id`    = (SELECT c.`chapter_id` FROM `novel_chapters` c WHERE c.`novel_id` = n.`novel_id` ORDER BY c.`chapter_num` DESC LIMIT 1),
    n.`latest_chapter_title` = (SELECT c.`title` FROM `novel_chapters` c WHERE c.`novel_id` = n.`novel_id` ORDER BY c.`chapter_num` DESC LIMIT 1),
    n.`latest_chapter_at`    = (SELECT c.`created_at` FROM `novel_chapters` c WHERE c.`novel_id` = n.`novel_id` ORDER BY c.`chapter_num` DESC LIMIT 1)
WHERE n.`novel_id` IN (9001, 9002, 9003);

-- ---- 演示评论（段评 / 章评 / 书评）----
-- 演示账号取库里真实存在的第一个用户，避免评论挂在不存在的 userID 上。
SET @demo_user = COALESCE((SELECT `userID` FROM `users` ORDER BY `userID` ASC LIMIT 1), 0);

INSERT IGNORE INTO `novel_comments`
  (`comment_id`, `novel_id`, `chapter_id`, `paragraph_index`, `target_type`, `paragraph_excerpt`, `user_id`, `content`, `like_num`, `created_at`)
VALUES
  (95001, 9001, 91001, 8, 'paragraph', '外婆说，人这一辈子只要看清一次银河，就不怕黑了。', @demo_user,
   '这句我读了三遍。我们宿舍楼停电那次大家都在聊天，没人抬头看天。', 3, NOW()),
  (95002, 9001, 91001, 6, 'paragraph', '停电持续了十一分钟。后来我数过，因为那十一分钟里我把银河记了一遍。', @demo_user,
   '十一分钟这个数字很真。真的事才会记得这么准。', 1, NOW()),
  (95003, 9001, 91002, 7, 'paragraph', '第三个月，她的第三张纸开始借给我。', @demo_user,
   '画箭头不写你错了——这个同桌我要了。', 2, NOW()),
  (95004, 9001, 91006, -1, 'chapter', '', @demo_user,
   '第六章看完有点难受。本子留在看台上这个结尾太好了。', 1, NOW()),
  (95005, 9001, 0, -1, 'book', '', @demo_user,
   '写得像我们自己的高中。建议后面把撕页那个人写清楚。', 4, NOW()),
  (95006, 9002, 92003, -1, 'chapter', '', @demo_user,
   '洞我找到了：跑步的男生走消防通道，可消防通道也要经过楼层，他说从不走楼梯这句本身可疑。', 2, NOW()),
  (95007, 9002, 92005, 11, 'paragraph', '除湿机的外壳是热的。铁扶手变温，是因为有人每晚握着它站在那台机器旁边。', @demo_user,
   '原来温度这条线在这儿收口。前面摸扶手那段埋得真早。', 3, NOW()),
  (95008, 9003, 93001, 8, 'paragraph', '重量不是克数，是一个人写这封信花了多久。', @demo_user,
   '扫描阵列读不出重量。这句可以当整本书的题眼。', 5, NOW()),
  (95009, 9003, 93004, -1, 'chapter', '', @demo_user,
   '第七号线不载人，但有人。完结这一下顶得住。', 2, NOW());

UPDATE `novels` n
SET n.`comment_num` = (SELECT COUNT(*) FROM `novel_comments` c WHERE c.`novel_id` = n.`novel_id`)
WHERE n.`novel_id` IN (9001, 9002, 9003);

UPDATE `novel_chapters` c
SET c.`comment_num` = (SELECT COUNT(*) FROM `novel_comments` n WHERE n.`chapter_id` = c.`chapter_id`)
WHERE c.`novel_id` IN (9001, 9002, 9003);

-- =============================================================================
-- 回滚（按需执行）
-- 种子：DELETE FROM novel_comment_likes WHERE comment_id BETWEEN 95001 AND 95099;
--       DELETE FROM novel_comments   WHERE comment_id BETWEEN 95001 AND 95099;
--       DELETE FROM novel_chapters   WHERE novel_id IN (9001, 9002, 9003);
--       DELETE FROM novels           WHERE novel_id IN (9001, 9002, 9003);
-- 全量：DROP TABLE IF EXISTS novel_progress, novel_bookshelf, novel_comment_likes,
--                     novel_comments, novel_chapters, novels;
-- =============================================================================
